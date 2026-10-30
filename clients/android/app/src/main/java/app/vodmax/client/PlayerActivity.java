package app.vodmax.client;

import android.app.Activity;
import android.content.Intent;
import android.content.res.ColorStateList;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.StateListDrawable;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.TextView;

import androidx.annotation.OptIn;
import androidx.media3.common.C;
import androidx.media3.common.MediaItem;
import androidx.media3.common.MediaMetadata;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.Player;
import androidx.media3.common.util.UnstableApi;
import androidx.media3.datasource.DefaultHttpDataSource;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory;
import androidx.media3.ui.DefaultTimeBar;
import androidx.media3.ui.PlayerView;

/**
 * Native playback (Media3): MP4, MKV, MPEG-TS and HLS with H.264/HEVC and
 * AAC/AC-3/E-AC-3 as the device supports them, with its audio and
 * subtitle tracks. Returns the position reached to the interface.
 *
 * Episodes: "Skip intro" while the opening plays and the next episode
 * near the end, as the interface knows them (learned per series); a
 * forward skip in the first minutes and "Next episode" during the credits
 * are returned, so that the interface learns them.
 */
@OptIn(markerClass = UnstableApi.class)
public class PlayerActivity extends Activity {
    static final String URL = "url", TITLE = "title", START = "start", LIVE = "live";
    static final String POSITION = "position", DURATION = "duration", ERROR = "error";
    static final String EPISODE = "episode", NEXT = "next", INTRO_START = "intro_start", INTRO_END = "intro_end", CREDITS = "credits";

    private static final int ACCENT = 0xFF00D6CA, PANEL = 0xEB13151B, LINE = 0x1FFFFFFF, TEXT = 0xFFF5F5F7, SOFT = 0xFFA9ABB5;

    private ExoPlayer player;
    private PlayerView view;
    private String error;

    // Episodes
    private boolean episode;
    private String nextTitle;
    private int introStart, introEnd, credits;          // known, seconds
    private int learnedIntroStart, learnedIntroEnd, learnedCredits;
    private boolean goNext, nextCancelled, skipping;
    private long nextAt;                                // ms, 0: not counting
    private long chainStart = -1, chainEnd;
    private Button skipIntro, nextButton;
    private LinearLayout nextCard;
    private TextView nextCount;
    private final Handler ticker = new Handler(Looper.getMainLooper());
    private final Runnable tick = new Runnable() {
        @Override
        public void run() {
            updateEpisodeExtras();
            ticker.postDelayed(this, 500);
        }
    };
    private final Runnable learnIntro = () -> {
        long start = chainStart, end = chainEnd;
        chainStart = -1;
        if (start <= 300_000 && end - start >= 20_000 && end <= 600_000) {
            learnedIntroStart = (int) (start / 1000);
            learnedIntroEnd = (int) (end / 1000);
            introStart = learnedIntroStart;
            introEnd = learnedIntroEnd;
        }
    };

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        Intent in = getIntent();
        episode = in.getBooleanExtra(EPISODE, false);
        nextTitle = in.getStringExtra(NEXT);
        if (nextTitle != null && nextTitle.isEmpty()) nextTitle = null;
        introStart = in.getIntExtra(INTRO_START, 0);
        introEnd = in.getIntExtra(INTRO_END, 0);
        credits = in.getIntExtra(CREDITS, 0);
        DefaultHttpDataSource.Factory http = new DefaultHttpDataSource.Factory()
            .setUserAgent("VodMaxAndroid/" + BuildConfig.VERSION_NAME)
            .setAllowCrossProtocolRedirects(true)
            .setConnectTimeoutMs(15000)
            .setReadTimeoutMs(30000);
        player = new ExoPlayer.Builder(this)
            .setMediaSourceFactory(new DefaultMediaSourceFactory(http))
            .setSeekBackIncrementMs(10000)
            .setSeekForwardIncrementMs(30000)
            .build();
        view = new PlayerView(this);
        view.setPlayer(player);
        view.setShowSubtitleButton(true);
        view.setShowNextButton(false);
        view.setShowPreviousButton(false);
        view.setKeepScreenOn(true);
        view.setControllerShowTimeoutMs(4000);
        DefaultTimeBar bar = view.findViewById(androidx.media3.ui.R.id.exo_progress);
        if (bar != null) {
            bar.setPlayedColor(ACCENT);
            bar.setScrubberColor(ACCENT);
            bar.setBufferedColor(0x55FFFFFF);
            bar.setUnplayedColor(0x33FFFFFF);
        }

        FrameLayout root = new FrameLayout(this);
        root.addView(view, new FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        if (episode) buildEpisodeExtras(root);
        setContentView(root);
        hideSystemBars();

        MediaItem item = new MediaItem.Builder()
            .setUri(Uri.parse(in.getStringExtra(URL)))
            .setMediaMetadata(new MediaMetadata.Builder().setTitle(in.getStringExtra(TITLE)).build())
            .build();
        long start = in.getLongExtra(START, 0);
        if (start > 0) player.setMediaItem(item, start);
        else player.setMediaItem(item);
        player.addListener(new Player.Listener() {
            @Override
            public void onPlaybackStateChanged(int state) {
                if (state != Player.STATE_ENDED) return;
                if (nextTitle != null && !nextCancelled) playNext(false);
                else finishWithResult();
            }

            @Override
            public void onPlayerError(PlaybackException e) {
                error = e.getErrorCodeName() + ": " + e.getMessage();
                finishWithResult();
            }

            @Override
            public void onPositionDiscontinuity(Player.PositionInfo from, Player.PositionInfo to, int reason) {
                if (reason == Player.DISCONTINUITY_REASON_SEEK) seeked(from.positionMs, to.positionMs);
            }
        });
        view.setControllerVisibilityListener((PlayerView.ControllerVisibilityListener) visibility -> updateEpisodeExtras());
        player.prepare();
        player.play();
        view.requestFocus();
        ticker.post(tick);
    }

    // ---- Episodes ----

    private int dp(float v) {
        return (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v, getResources().getDisplayMetrics());
    }

    /** A pill like the buttons of the interface: white when focused. */
    private Button pill(int text, boolean bold) {
        Button b = new Button(this);
        b.setText(text);
        b.setAllCaps(false);
        b.setTextSize(TypedValue.COMPLEX_UNIT_SP, 16);
        if (bold) b.setTypeface(Typeface.DEFAULT_BOLD);
        b.setPadding(dp(20), dp(8), dp(20), dp(8));
        b.setMinHeight(0);
        b.setMinimumHeight(0);
        GradientDrawable on = new GradientDrawable(), off = new GradientDrawable();
        on.setCornerRadius(dp(999));
        on.setColor(Color.WHITE);
        off.setCornerRadius(dp(999));
        off.setColor(PANEL);
        off.setStroke(dp(1), LINE);
        StateListDrawable bg = new StateListDrawable();
        bg.addState(new int[] {android.R.attr.state_focused}, on);
        bg.addState(new int[] {android.R.attr.state_pressed}, on);
        bg.addState(new int[] {}, off);
        b.setBackground(bg);
        b.setTextColor(new ColorStateList(new int[][] {{android.R.attr.state_focused}, {android.R.attr.state_pressed}, {}},
            new int[] {0xFF0B0C10, 0xFF0B0C10, TEXT}));
        b.setStateListAnimator(null);
        b.setFocusable(true);
        return b;
    }

    private TextView text(String s, int sp, int color, boolean bold) {
        TextView t = new TextView(this);
        t.setText(s);
        t.setTextSize(TypedValue.COMPLEX_UNIT_SP, sp);
        t.setTextColor(color);
        if (bold) t.setTypeface(Typeface.DEFAULT_BOLD);
        t.setSingleLine(true);
        t.setEllipsize(android.text.TextUtils.TruncateAt.END);
        return t;
    }

    private void buildEpisodeExtras(FrameLayout root) {
        LinearLayout box = new LinearLayout(this);
        box.setOrientation(LinearLayout.VERTICAL);
        box.setGravity(Gravity.END);
        FrameLayout.LayoutParams lp = new FrameLayout.LayoutParams(FrameLayout.LayoutParams.WRAP_CONTENT,
            FrameLayout.LayoutParams.WRAP_CONTENT, Gravity.BOTTOM | Gravity.END);
        lp.setMargins(dp(24), dp(24), dp(32), dp(32));
        root.addView(box, lp);

        skipIntro = pill(R.string.skip_intro, true);
        skipIntro.setVisibility(View.GONE);
        skipIntro.setOnClickListener(v -> {
            skipping = true;
            player.seekTo(introEnd * 1000L);
            skipping = false;
            hide(skipIntro);
        });
        box.addView(skipIntro);

        nextButton = pill(R.string.next_episode, false);
        nextButton.setVisibility(View.GONE);
        nextButton.setOnClickListener(v -> playNext(true));
        box.addView(nextButton);

        nextCard = new LinearLayout(this);
        nextCard.setOrientation(LinearLayout.VERTICAL);
        nextCard.setPadding(dp(20), dp(16), dp(20), dp(16));
        GradientDrawable card = new GradientDrawable();
        card.setCornerRadius(dp(16));
        card.setColor(PANEL);
        card.setStroke(dp(1), LINE);
        nextCard.setBackground(card);
        nextCard.setVisibility(View.GONE);
        TextView label = text(getString(R.string.next_episode).toUpperCase(), 13, ACCENT, true);
        label.setLetterSpacing(0.06f);
        nextCard.addView(label);
        nextCard.addView(text(nextTitle == null ? "" : nextTitle, 17, TEXT, true));
        nextCount = text("", 14, SOFT, false);
        nextCard.addView(nextCount);
        LinearLayout buttons = new LinearLayout(this);
        buttons.setPadding(0, dp(10), 0, 0);
        Button now = pill(R.string.watch_now, false), cancel = pill(R.string.cancel, false);
        now.setOnClickListener(v -> playNext(true));
        cancel.setOnClickListener(v -> {
            nextCancelled = true;
            hide(nextCard);
        });
        buttons.addView(now);
        LinearLayout.LayoutParams gap = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        gap.setMarginStart(dp(8));
        buttons.addView(cancel, gap);
        nextCard.addView(buttons);
        LinearLayout.LayoutParams cardLp = new LinearLayout.LayoutParams(dp(360), LinearLayout.LayoutParams.WRAP_CONTENT);
        cardLp.topMargin = dp(12);
        box.addView(nextCard, cardLp);
    }

    /** Shows v and gives it the focus, so that OK on the remote presses it. */
    private void reveal(View v) {
        if (v.getVisibility() == View.VISIBLE) return;
        v.setVisibility(View.VISIBLE);
        if (view.isControllerFullyVisible()) return;
        View target = v instanceof LinearLayout ? v.findFocus() == null ? firstButton((LinearLayout) v) : v.findFocus() : v;
        if (target != null) target.requestFocus();
    }

    private View firstButton(LinearLayout l) {
        for (int i = 0; i < l.getChildCount(); i++) {
            View c = l.getChildAt(i);
            if (c instanceof Button) return c;
            if (c instanceof LinearLayout) {
                View b = firstButton((LinearLayout) c);
                if (b != null) return b;
            }
        }
        return null;
    }

    private void hide(View v) {
        if (v.getVisibility() != View.VISIBLE) return;
        boolean had = v.hasFocus();
        v.setVisibility(View.GONE);
        if (had) view.requestFocus();
    }

    private void updateEpisodeExtras() {
        if (player == null || skipIntro == null) return;
        long pos = player.getCurrentPosition(), d = player.getDuration();
        if (introEnd > 0 && pos >= introStart * 1000L && pos < introEnd * 1000L - 2000) reveal(skipIntro);
        else hide(skipIntro);
        if (nextTitle == null || d == C.TIME_UNSET || d <= 0 || nextCancelled) {
            hide(nextCard);
            hide(nextButton);
            return;
        }
        // With the credits known the card comes when they start and plays
        // the next one 10 s later; otherwise it comes 15 s before the end.
        long left = d - pos;
        if (left > (credits > 0 ? credits : 15) * 1000L) {
            nextAt = 0;
            hide(nextCard);
            if (view.isControllerFullyVisible()) nextButton.setVisibility(View.VISIBLE);
            else hide(nextButton);
            return;
        }
        hide(nextButton);
        if (nextAt == 0) nextAt = credits > 0 ? Math.min(d, pos + 10_000) : d;
        nextCount.setText(getString(R.string.next_in, (int) Math.max(0, Math.ceil((nextAt - pos) / 1000.0))));
        reveal(nextCard);
        if (pos >= nextAt && nextAt < d) playNext(false);
    }

    /** Forward seeks close together are one skip; see learnIntro. */
    private void seeked(long from, long to) {
        if (skipping || !episode) return;
        ticker.removeCallbacks(learnIntro);
        if (to <= from) {
            chainStart = -1;
            return;
        }
        if (chainStart < 0) chainStart = from;
        chainEnd = to;
        ticker.postDelayed(learnIntro, 8000);
    }

    /** byPerson: chosen during the credits, which then are this long. */
    private void playNext(boolean byPerson) {
        if (player == null || goNext) return;
        long d = player.getDuration(), left = d == C.TIME_UNSET ? 0 : d - player.getCurrentPosition();
        if (byPerson && left >= 15_000 && left <= 600_000) learnedCredits = (int) (left / 1000);
        goNext = true;
        finishWithResult();
    }

    private void hideSystemBars() {
        View d = getWindow().getDecorView();
        if (android.os.Build.VERSION.SDK_INT >= 30) {
            WindowInsetsController c = d.getWindowInsetsController();
            if (c != null) {
                c.hide(WindowInsets.Type.systemBars());
                c.setSystemBarsBehavior(WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
            }
        } else {
            d.setSystemUiVisibility(View.SYSTEM_UI_FLAG_FULLSCREEN | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);
        }
    }

    // Remote: OK and the arrows show the controls (Media3 handles seeking
    // and the track menu); Back closes the player. On "Skip intro" or the
    // next-episode card, the keys move among their buttons and OK presses.
    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        if (event.getKeyCode() == KeyEvent.KEYCODE_BACK) {
            if (event.getAction() == KeyEvent.ACTION_UP) finishWithResult();
            return true;
        }
        View f = getCurrentFocus();
        if (f instanceof Button && (f == skipIntro || f == nextButton || (nextCard != null && f.getParent() != null
                && f.getParent().getParent() == nextCard))) {
            return super.dispatchKeyEvent(event);
        }
        return view.dispatchKeyEvent(event) || super.dispatchKeyEvent(event);
    }

    private void finishWithResult() {
        if (player == null) return;
        ticker.removeCallbacksAndMessages(null);
        if (chainStart >= 0) learnIntro.run();
        Intent out = new Intent();
        long position = player.getCurrentPosition();
        long duration = player.getDuration();
        out.putExtra(POSITION, Math.max(0, position));
        out.putExtra(DURATION, duration == C.TIME_UNSET ? 0 : duration);
        if (error != null) out.putExtra(ERROR, error);
        out.putExtra(NEXT, goNext);
        if (learnedIntroEnd > 0) {
            out.putExtra(INTRO_START, learnedIntroStart);
            out.putExtra(INTRO_END, learnedIntroEnd);
        }
        if (learnedCredits > 0) out.putExtra(CREDITS, learnedCredits);
        setResult(RESULT_OK, out);
        player.release();
        player = null;
        finish();
    }

    @Override
    protected void onStop() {
        super.onStop();
        if (player != null && !isFinishing()) player.pause();
    }

    @Override
    protected void onDestroy() {
        ticker.removeCallbacksAndMessages(null);
        if (player != null) {
            player.release();
            player = null;
        }
        super.onDestroy();
    }
}
