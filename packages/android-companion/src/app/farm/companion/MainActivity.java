package app.farm.companion;

import android.app.Activity;
import android.content.Intent;
import android.os.Bundle;
import android.provider.Settings;
import android.view.Gravity;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;

/** A setup page in a separate process so the service can read it normally. */
public final class MainActivity extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        int padding = Math.round(24 * getResources().getDisplayMetrics().density);
        layout.setPadding(padding, padding, padding, padding);
        layout.setGravity(Gravity.CENTER_VERTICAL);
        TextView title = new TextView(this);
        title.setText("Farm Companion");
        title.setTextSize(24);
        layout.addView(title);
        TextView description = new TextView(this);
        description.setText("为 Farm 加速读取当前界面控件树。\n\n仅提供只读无障碍快照，不注入点击、不读取通知、不联网。请在无障碍设置中启用 Farm Companion；停用后 Farm 仍可通过 UIAutomator 读取。\n");
        description.setTextSize(16);
        layout.addView(description);
        Button settings = new Button(this);
        settings.setText("打开无障碍设置");
        settings.setOnClickListener(view -> startActivity(new Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)));
        layout.addView(settings);
        setContentView(layout);
    }
}
