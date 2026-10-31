package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silirim/vodmax/internal/store"
)

// Sign-in with an account of the house (Authelia, Authentik, Keycloak,
// Google...) through OpenID Connect: a browser that opens VodMax can enter
// with that account instead of a pairing code. Each account is linked by
// the admin to one profile; whoever enters gets a device locked to it. The
// ID token comes straight from the token endpoint over HTTPS, as OpenID
// Connect allows (Core 3.1.3.7), so its issuer, audience, expiry and nonce
// are checked but not its signature.

const (
	ssoKey       = "sso"
	ssoSecretKey = "sso_secret"
	ssoLoginTTL  = 10 * time.Minute
	ssoClaimTTL  = 2 * time.Minute
	ssoMaxLogins = 100
)

var (
	ErrSSOOff     = errors.New("l'accesso con account esterno non è configurato")
	ErrSSOLogin   = errors.New("accesso non riuscito: riprova dall'inizio")
	ErrSSOAccount = errors.New("questo account non è collegato a nessun profilo di VodMax: chiedi a chi gestisce il server")
)

// SSOAccount links an account (its e-mail or username) to a profile.
type SSOAccount struct {
	Account   string `json:"account"`
	ProfileID int64  `json:"profile_id"`
}

// SSOSettings is the configuration; the client secret is kept apart,
// sealed, and never returned.
type SSOSettings struct {
	Enabled  bool         `json:"enabled"`
	Issuer   string       `json:"issuer"`
	ClientID string       `json:"client_id"`
	Label    string       `json:"label"`
	Accounts []SSOAccount `json:"accounts"`
}

type ssoState struct {
	mu     sync.Mutex
	logins map[string]ssoLogin // by state
	claims map[string]ssoClaim // by one-time code
	disc   *ssoDiscovery
	client *http.Client // tests
}

type ssoLogin struct {
	verifier, nonce, redirect string
	expires                   time.Time
}

type ssoClaim struct {
	token, profile string
	expires        time.Time
}

type ssoDiscovery struct {
	configured    string // the issuer as configured
	Issuer        string `json:"issuer"`
	AuthEndpoint  string `json:"authorization_endpoint"`
	TokenEndpoint string `json:"token_endpoint"`
	fetched       time.Time
}

func (s *Service) SSOSettings(ctx context.Context) (SSOSettings, bool, error) {
	var o SSOSettings
	if err := s.st.Setting(ctx, ssoKey, &o); err != nil {
		return o, false, err
	}
	if o.Accounts == nil {
		o.Accounts = []SSOAccount{}
	}
	secret, err := s.secretSetting(ctx, ssoSecretKey)
	return o, secret != "", err
}

// SSOInput changes the configuration; Secret nil keeps the current one.
type SSOInput struct {
	Enabled  bool         `json:"enabled"`
	Issuer   string       `json:"issuer"`
	ClientID string       `json:"client_id"`
	Secret   *string      `json:"client_secret"`
	Label    string       `json:"label"`
	Accounts []SSOAccount `json:"accounts"`
}

func (s *Service) SetSSO(ctx context.Context, in SSOInput) error {
	o := SSOSettings{Enabled: in.Enabled, Issuer: strings.TrimRight(strings.TrimSpace(in.Issuer), "/"),
		ClientID: strings.TrimSpace(in.ClientID), Label: strings.TrimSpace(in.Label)}
	if len(o.Label) > 30 {
		return invalid("nome del pulsante troppo lungo (max 30 caratteri)")
	}
	if o.Issuer != "" {
		u, err := url.Parse(o.Issuer)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return invalid("l'indirizzo del provider di accesso deve iniziare con https://")
		}
	}
	profiles, err := s.st.Profiles(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, a := range in.Accounts {
		a.Account = strings.ToLower(strings.TrimSpace(a.Account))
		if a.Account == "" {
			continue
		}
		if seen[a.Account] {
			return invalid("l'account %s compare due volte", a.Account)
		}
		if !slices.ContainsFunc(profiles, func(p store.Profile) bool { return p.ID == a.ProfileID }) {
			return invalid("profilo inesistente per %s", a.Account)
		}
		seen[a.Account] = true
		o.Accounts = append(o.Accounts, a)
	}
	if in.Secret != nil {
		if err := s.setSecretSetting(ctx, ssoSecretKey, strings.TrimSpace(*in.Secret)); err != nil {
			return err
		}
	}
	secret, err := s.secretSetting(ctx, ssoSecretKey)
	if err != nil {
		return err
	}
	if o.Enabled && (o.Issuer == "" || o.ClientID == "" || secret == "") {
		return invalid("per accendere l'accesso servono indirizzo del provider, client ID e client secret")
	}
	if o.Enabled && len(o.Accounts) == 0 {
		return invalid("collega almeno un account a un profilo")
	}
	if err := s.st.SetSetting(ctx, ssoKey, o); err != nil {
		return err
	}
	s.sso.mu.Lock()
	s.sso.disc = nil
	s.sso.mu.Unlock()
	return nil
}

// SSOButton is what the pairing screen shows: whether to offer the sign-in
// and the name of the button.
func (s *Service) SSOButton(ctx context.Context) (bool, string) {
	o, _, err := s.SSOSettings(ctx)
	if err != nil || !o.Enabled {
		return false, ""
	}
	return true, o.Label
}

func (s *Service) ssoClient() *http.Client {
	if s.sso.client != nil {
		return s.sso.client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (s *Service) ssoDiscover(ctx context.Context, issuer string) (*ssoDiscovery, error) {
	s.sso.mu.Lock()
	d := s.sso.disc
	s.sso.mu.Unlock()
	if d != nil && d.configured == issuer && time.Since(d.fetched) < time.Hour {
		return d, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.ssoClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider di accesso non raggiungibile: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider di accesso: risposta %d all'indirizzo di configurazione", resp.StatusCode)
	}
	d = &ssoDiscovery{configured: issuer, fetched: time.Now()}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(d); err != nil {
		return nil, fmt.Errorf("provider di accesso: configurazione illeggibile: %w", err)
	}
	for _, e := range []string{d.AuthEndpoint, d.TokenEndpoint} {
		if u, err := url.Parse(e); err != nil || u.Scheme != "https" {
			return nil, errors.New("provider di accesso: gli indirizzi devono essere https://")
		}
	}
	if strings.TrimRight(d.Issuer, "/") != issuer {
		return nil, errors.New("provider di accesso: l'indirizzo dichiarato non corrisponde a quello configurato")
	}
	s.sso.mu.Lock()
	s.sso.disc = d
	s.sso.mu.Unlock()
	return d, nil
}

// SSOStart returns the address of the provider's sign-in page; redirect is
// VodMax's callback address, which the provider must know.
func (s *Service) SSOStart(ctx context.Context, redirect string) (string, error) {
	o, _, err := s.SSOSettings(ctx)
	if err != nil {
		return "", err
	}
	if !o.Enabled {
		return "", ErrSSOOff
	}
	d, err := s.ssoDiscover(ctx, o.Issuer)
	if err != nil {
		return "", err
	}
	state, verifier, nonce := randomToken(), randomToken(), randomToken()
	s.sso.mu.Lock()
	if s.sso.logins == nil {
		s.sso.logins = map[string]ssoLogin{}
	}
	now := time.Now()
	for k, l := range s.sso.logins {
		if now.After(l.expires) {
			delete(s.sso.logins, k)
		}
	}
	if len(s.sso.logins) >= ssoMaxLogins {
		s.sso.mu.Unlock()
		return "", ErrBusy
	}
	s.sso.logins[state] = ssoLogin{verifier: verifier, nonce: nonce, redirect: redirect, expires: now.Add(ssoLoginTTL)}
	s.sso.mu.Unlock()
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {o.ClientID}, "redirect_uri": {redirect},
		"scope": {"openid email profile"}, "state": {state}, "nonce": {nonce},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}}
	sep := "?"
	if strings.Contains(d.AuthEndpoint, "?") {
		sep = "&"
	}
	return d.AuthEndpoint + sep + q.Encode(), nil
}

type ssoClaims struct {
	Iss           string          `json:"iss"`
	Sub           string          `json:"sub"`
	Aud           json.RawMessage `json:"aud"`
	Exp           int64           `json:"exp"`
	Nonce         string          `json:"nonce"`
	Email         string          `json:"email"`
	EmailVerified *bool           `json:"email_verified"`
	Username      string          `json:"preferred_username"`
	Name          string          `json:"name"`
}

func (c ssoClaims) audience(clientID string) bool {
	var one string
	if json.Unmarshal(c.Aud, &one) == nil {
		return one == clientID
	}
	var many []string
	return json.Unmarshal(c.Aud, &many) == nil && slices.Contains(many, clientID)
}

// SSOFinish completes the sign-in the provider sent back: it pairs a new
// device locked to the linked profile and returns a one-time code with
// which the browser collects its tokens.
func (s *Service) SSOFinish(ctx context.Context, state, code string) (string, error) {
	s.sso.mu.Lock()
	l, ok := s.sso.logins[state]
	delete(s.sso.logins, state)
	s.sso.mu.Unlock()
	if !ok || time.Now().After(l.expires) || code == "" {
		return "", ErrSSOLogin
	}
	o, _, err := s.SSOSettings(ctx)
	if err != nil {
		return "", err
	}
	if !o.Enabled {
		return "", ErrSSOOff
	}
	secret, err := s.secretSetting(ctx, ssoSecretKey)
	if err != nil {
		return "", err
	}
	d, err := s.ssoDiscover(ctx, o.Issuer)
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {l.redirect}, "code_verifier": {l.verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(url.QueryEscape(o.ClientID), url.QueryEscape(secret))
	resp, err := s.ssoClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("provider di accesso non raggiungibile: %w", err)
	}
	defer resp.Body.Close()
	var tok struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tok)
	if resp.StatusCode != http.StatusOK || tok.IDToken == "" {
		return "", fmt.Errorf("il provider di accesso ha rifiutato il codice (%d %s)", resp.StatusCode, tok.Error)
	}
	parts := strings.Split(tok.IDToken, ".")
	if len(parts) != 3 {
		return "", ErrSSOLogin
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return "", ErrSSOLogin
	}
	var c ssoClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", ErrSSOLogin
	}
	if strings.TrimRight(c.Iss, "/") != o.Issuer || !c.audience(o.ClientID) || time.Now().Unix() > c.Exp || c.Nonce != l.nonce || c.Sub == "" {
		return "", ErrSSOLogin
	}
	profile, who := ssoMatch(o.Accounts, c)
	if profile == 0 {
		return "", ErrSSOAccount
	}
	p, err := s.st.Profile(ctx, profile)
	if err != nil {
		return "", ErrSSOAccount
	}
	token := randomToken()
	name := "Accesso " + who
	if len(name) > 60 {
		name = name[:60]
	}
	id, err := s.st.AddDevice(ctx, name, hashToken(token))
	if err != nil {
		return "", err
	}
	if err := s.st.UpdateDevice(ctx, id, name, &p.ID); err != nil {
		return "", err
	}
	once := randomToken()
	s.sso.mu.Lock()
	if s.sso.claims == nil {
		s.sso.claims = map[string]ssoClaim{}
	}
	for k, c := range s.sso.claims {
		if time.Now().After(c.expires) {
			delete(s.sso.claims, k)
		}
	}
	s.sso.claims[once] = ssoClaim{token: token, profile: strconv.FormatInt(p.ID, 10) + "." + s.profileSignature(id, p), expires: time.Now().Add(ssoClaimTTL)}
	s.sso.mu.Unlock()
	return once, nil
}

// ssoMatch finds the linked profile: by verified e-mail, then by username.
func ssoMatch(accounts []SSOAccount, c ssoClaims) (int64, string) {
	email := strings.ToLower(c.Email)
	user := strings.ToLower(c.Username)
	for _, a := range accounts {
		if email != "" && (c.EmailVerified == nil || *c.EmailVerified) && a.Account == email {
			return a.ProfileID, email
		}
	}
	for _, a := range accounts {
		if user != "" && a.Account == user {
			return a.ProfileID, user
		}
	}
	return 0, ""
}

// SSOClaim hands the device and profile tokens out, once.
func (s *Service) SSOClaim(code string) (token, profile string, err error) {
	s.sso.mu.Lock()
	defer s.sso.mu.Unlock()
	c, ok := s.sso.claims[code]
	delete(s.sso.claims, code)
	if !ok || time.Now().After(c.expires) {
		return "", "", ErrSSOLogin
	}
	return c.token, c.profile, nil
}

// SetSSOClient replaces the HTTP client towards the sign-in provider (tests).
func (s *Service) SetSSOClient(c *http.Client) { s.sso.client = c }
