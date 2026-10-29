package app.vodmax.client;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Bundle;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowManager;
import android.view.inputmethod.EditorInfo;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.EditText;
import android.widget.TextView;

import org.json.JSONObject;

import java.net.HttpURLConnection;
import java.net.URL;

/**
 * The VodMax app: the server's interface in a WebView (the same one as in
 * the browser), with films, episodes, recordings and catch-up played by
 * the native player (Media3), which reads formats a browser cannot.
 * First run: the server address; the device is then paired with a code in
 * the interface itself.
 */
public class MainActivity extends Activity {
    private static final String PREFS = "vodmax";
    private static final String KEY_SERVER = "server";
    private static final int PLAY = 1;

    private WebView web;
    private String server;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        server = prefs().getString(KEY_SERVER, null);
        if (server == null) showSetup(null);
        else open();
    }

    private SharedPreferences prefs() {
        return getSharedPreferences(PREFS, MODE_PRIVATE);
    }

    // ---- First run: the server address ----

    private void showSetup(String message) {
        if (web != null) {
            web.destroy();
            web = null;
        }
        setContentView(R.layout.setup);
        EditText field = findViewById(R.id.server);
        TextView text = findViewById(R.id.setup_text);
        Button connect = findViewById(R.id.connect);
        if (server != null) field.setText(server);
        if (message != null) text.setText(message);
        Runnable go = () -> {
            String url = normalize(field.getText().toString());
            if (url == null) return;
            connect.setEnabled(false);
            text.setText(R.string.setup_checking);
            new Thread(() -> {
                boolean ok = reachable(url);
                runOnUiThread(() -> {
                    connect.setEnabled(true);
                    if (!ok) {
                        text.setText(R.string.setup_unreachable);
                        return;
                    }
                    server = url;
                    prefs().edit().putString(KEY_SERVER, url).apply();
                    open();
                });
            }).start();
        };
        connect.setOnClickListener(v -> go.run());
        field.setOnEditorActionListener((v, action, event) -> {
            if (action == EditorInfo.IME_ACTION_GO || action == EditorInfo.IME_ACTION_DONE) {
                go.run();
                return true;
            }
            return false;
        });
        field.requestFocus();
    }

    /** "192.168.1.10:8095" → "http://192.168.1.10:8095"; null if not an address. */
    static String normalize(String s) {
        s = s.trim();
        if (s.isEmpty()) return null;
        if (!s.startsWith("http://") && !s.startsWith("https://")) s = "http://" + s;
        while (s.endsWith("/")) s = s.substring(0, s.length() - 1);
        Uri u = Uri.parse(s);
        return u.getHost() == null || u.getHost().isEmpty() ? null : s;
    }

    private static boolean reachable(String base) {
        try {
            HttpURLConnection c = (HttpURLConnection) new URL(base + "/api/v1/health").openConnection();
            c.setConnectTimeout(5000);
            c.setReadTimeout(5000);
            int code = c.getResponseCode();
            c.disconnect();
            return code == 200;
        } catch (Exception e) {
            return false;
        }
    }

    // ---- The interface ----

    @SuppressLint("SetJavaScriptEnabled")
    private void open() {
        web = new WebView(this);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setMediaPlaybackRequiresUserGesture(false);
        s.setUserAgentString(s.getUserAgentString() + " VodMaxAndroid/" + BuildConfig.VERSION_NAME);
        web.setBackgroundColor(0xFF0B0C10);
        web.addJavascriptInterface(new Bridge(), "VodMaxNative");
        web.setWebChromeClient(new WebChromeClient());
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                Uri u = request.getUrl();
                if (u.toString().startsWith(server)) return false;
                // Trailers and other sites open outside the app.
                try {
                    startActivity(new Intent(Intent.ACTION_VIEW, u));
                } catch (Exception ignored) {
                }
                return true;
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                if (request.isForMainFrame()) showOffline();
            }
        });
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        setContentView(web);
        web.requestFocus(View.FOCUS_DOWN);
        web.loadUrl(server + "/");
    }

    private void showOffline() {
        showSetup(getString(R.string.error_title) + ": " + server + ". " + getString(R.string.setup_unreachable));
    }

    /** Called by the page: window.VodMaxNative. */
    private class Bridge {
        @JavascriptInterface
        public String version() {
            return BuildConfig.VERSION_NAME;
        }

        /** {url, title, start (s), live} → the native player. */
        @JavascriptInterface
        public void play(String json) {
            runOnUiThread(() -> {
                try {
                    JSONObject o = new JSONObject(json);
                    Intent i = new Intent(MainActivity.this, PlayerActivity.class);
                    String url = o.getString("url");
                    if (url.startsWith("/")) url = server + url;
                    i.putExtra(PlayerActivity.URL, url);
                    i.putExtra(PlayerActivity.TITLE, o.optString("title"));
                    i.putExtra(PlayerActivity.START, (long) (o.optDouble("start", 0) * 1000));
                    i.putExtra(PlayerActivity.LIVE, o.optBoolean("live", false));
                    startActivityForResult(i, PLAY);
                } catch (Exception e) {
                    web.evaluateJavascript("window.VodMaxNativeError && window.VodMaxNativeError("
                        + JSONObject.quote(String.valueOf(e.getMessage())) + ")", null);
                }
            });
        }

        @JavascriptInterface
        public void changeServer() {
            runOnUiThread(() -> showSetup(null));
        }
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request != PLAY || web == null) return;
        long position = data == null ? 0 : data.getLongExtra(PlayerActivity.POSITION, 0);
        long duration = data == null ? 0 : data.getLongExtra(PlayerActivity.DURATION, 0);
        String error = data == null ? null : data.getStringExtra(PlayerActivity.ERROR);
        web.evaluateJavascript("window.VodMaxNativeDone && window.VodMaxNativeDone({position:" + position / 1000.0
            + ",duration:" + duration / 1000.0 + ",error:" + (error == null ? "null" : JSONObject.quote(error)) + "})", null);
    }

    // Back: the page decides (close a panel, go to the menu); with nothing
    // to close, the app goes to the background.
    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        if (keyCode == KeyEvent.KEYCODE_BACK && web != null) {
            web.evaluateJavascript("(window.VodMaxNativeBack ? window.VodMaxNativeBack() : false)", handled -> {
                if (!"true".equals(handled)) moveTaskToBack(true);
            });
            return true;
        }
        return super.onKeyDown(keyCode, event);
    }

    @Override
    protected void onDestroy() {
        if (web != null) web.destroy();
        super.onDestroy();
    }
}
