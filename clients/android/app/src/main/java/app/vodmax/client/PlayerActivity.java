package app.vodmax.client;

import android.app.Activity;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.WindowManager;

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
import androidx.media3.ui.PlayerView;

/**
 * Native playback (Media3): MP4, MKV, MPEG-TS and HLS with H.264/HEVC and
 * AAC/AC-3/E-AC-3 as the device supports them, with its audio and
 * subtitle tracks. Returns the position reached to the interface.
 */
public class PlayerActivity extends Activity {
    static final String URL = "url", TITLE = "title", START = "start", LIVE = "live";
    static final String POSITION = "position", DURATION = "duration", ERROR = "error";

    private ExoPlayer player;
    private PlayerView view;
    private String error;

    @Override
    @OptIn(markerClass = UnstableApi.class)
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        Intent in = getIntent();
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
        view.setKeepScreenOn(true);
        view.setControllerShowTimeoutMs(4000);
        setContentView(view);
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
                if (state == Player.STATE_ENDED) finishWithResult();
            }

            @Override
            public void onPlayerError(PlaybackException e) {
                error = e.getErrorCodeName() + ": " + e.getMessage();
                finishWithResult();
            }
        });
        player.prepare();
        player.play();
        view.requestFocus();
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
    // and the track menu); Back closes the player.
    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        if (event.getKeyCode() == KeyEvent.KEYCODE_BACK) {
            if (event.getAction() == KeyEvent.ACTION_UP) finishWithResult();
            return true;
        }
        return view.dispatchKeyEvent(event) || super.dispatchKeyEvent(event);
    }

    private void finishWithResult() {
        if (player == null) return;
        Intent out = new Intent();
        long position = player.getCurrentPosition();
        long duration = player.getDuration();
        out.putExtra(POSITION, Math.max(0, position));
        out.putExtra(DURATION, duration == C.TIME_UNSET ? 0 : duration);
        if (error != null) out.putExtra(ERROR, error);
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
        if (player != null) {
            player.release();
            player = null;
        }
        super.onDestroy();
    }
}
