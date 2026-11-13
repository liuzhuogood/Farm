package app.farm.companion;

import android.accessibilityservice.AccessibilityService;
import android.app.KeyguardManager;
import android.content.Intent;
import android.graphics.Point;
import android.graphics.Rect;
import android.hardware.display.DisplayManager;
import android.net.Credentials;
import android.net.LocalServerSocket;
import android.net.LocalSocket;
import android.os.Build;
import android.os.Process;
import android.os.SystemClock;
import android.system.Os;
import android.system.OsConstants;
import android.system.StructTimeval;
import android.util.Log;
import android.util.SparseArray;
import android.util.Xml;
import android.view.Display;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityNodeInfo;
import android.view.accessibility.AccessibilityWindowInfo;

import org.json.JSONObject;
import org.xmlpull.v1.XmlSerializer;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Set;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.Future;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.SynchronousQueue;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

/** Independently implemented read-only accessibility snapshot transport. */
public final class FarmAccessibilityService extends AccessibilityService {
    private static final String TAG = "FarmCompanion";
    private static final String SOCKET = "farm_companion";
    private static final int MAX_XML = 2 << 20;
    private static final int MAX_NODES = 5000;
    private static final int MAX_DEPTH = 64;
    private static final int REQUEST_LIMIT = 1024;
    private static final int IO_TIMEOUT_MS = 3000;
    private static final int SNAPSHOT_TIMEOUT_MS = 2000;

    private volatile boolean running;
    private volatile LocalServerSocket server;
    private final Set<LocalSocket> sockets = ConcurrentHashMap.newKeySet();
    private Thread acceptThread;
    private ThreadPoolExecutor connections;
    private ThreadPoolExecutor snapshots;

    @Override protected void onServiceConnected() {
        super.onServiceConnected();
        stopServer();
        connections = new ThreadPoolExecutor(2, 2, 0, TimeUnit.MILLISECONDS,
            new SynchronousQueue<Runnable>(), runnable -> daemon(runnable, "farm-client"));
        snapshots = new ThreadPoolExecutor(1, 1, 0, TimeUnit.MILLISECONDS,
            new ArrayBlockingQueue<Runnable>(1), runnable -> daemon(runnable, "farm-tree"));
        running = true;
        acceptThread = daemon(this::acceptLoop, "farm-accept");
        acceptThread.start();
    }

    private static Thread daemon(Runnable runnable, String name) {
        Thread thread = new Thread(runnable, name);
        thread.setDaemon(true);
        return thread;
    }

    private void acceptLoop() {
        LocalServerSocket listener = null;
        try {
            listener = new LocalServerSocket(SOCKET);
            server = listener;
            if (!running) return;
            Log.i(TAG, "read-only service listening on @" + SOCKET);
            while (running) {
                LocalSocket socket = listener.accept();
                if (!running) { close(socket); break; }
                try {
                    Credentials peer = socket.getPeerCredentials();
                    int uid = peer.getUid();
                    if (uid != 0 && uid != 2000 && uid != Process.myUid()) {
                        Log.w(TAG, "rejected peer uid=" + uid);
                        close(socket);
                        continue;
                    }
                    socket.setSoTimeout(IO_TIMEOUT_MS);
                    Os.setsockoptTimeval(socket.getFileDescriptor(), OsConstants.SOL_SOCKET,
                        OsConstants.SO_SNDTIMEO, StructTimeval.fromMillis(IO_TIMEOUT_MS));
                    sockets.add(socket);
                    connections.execute(() -> handle(socket));
                } catch (RejectedExecutionException busy) {
                    sockets.remove(socket);
                    close(socket);
                    Log.w(TAG, "connection capacity reached");
                } catch (Exception clientError) {
                    sockets.remove(socket);
                    close(socket);
                    if (running) Log.w(TAG, "client setup failed: " + clientError.getClass().getSimpleName());
                }
            }
        } catch (Exception error) {
            if (running) Log.e(TAG, "socket server failed: " + error.getClass().getSimpleName());
        } finally {
            if (listener != null) try { listener.close(); } catch (IOException ignored) { }
            if (server == listener) server = null;
        }
    }

    private void handle(LocalSocket socket) {
        try {
            InputStream input = socket.getInputStream();
            OutputStream output = socket.getOutputStream();
            while (running) {
                String request = readLine(input, socket);
                if (request == null) break;
                JSONObject response;
                try {
                    JSONObject json = new JSONObject(request);
                    response = CompanionProtocol.dispatch(json.get("version"), json.get("command"),
                        FarmAccessibilityService::pong, this::snapshot);
                } catch (CompanionProtocol.UnsupportedRequest unsupported) {
                    response = error("unsupported_protocol_or_command");
                } catch (Exception invalid) {
                    response = error("invalid_request");
                }
                byte[] encoded = (response.toString() + "\n").getBytes(StandardCharsets.UTF_8);
                output.write(encoded);
                output.flush();
            }
        } catch (IOException disconnected) {
            // Read and send timeouts close this one client; the next may retry.
        } finally {
            sockets.remove(socket);
            close(socket);
        }
    }

    private static String readLine(InputStream input, LocalSocket socket) throws IOException {
        ByteArrayOutputStream line = new ByteArrayOutputStream();
        long deadline = SystemClock.elapsedRealtime() + IO_TIMEOUT_MS;
        while (true) {
            long remaining = deadline - SystemClock.elapsedRealtime();
            if (remaining <= 0) throw new IOException("request_timeout");
            socket.setSoTimeout((int) remaining);
            int value = input.read();
            if (value < 0) return line.size() == 0 ? null : throwIncomplete();
            if (value == '\n') return new String(line.toByteArray(), StandardCharsets.UTF_8);
            if (line.size() >= REQUEST_LIMIT) throw new IOException("request_limit");
            line.write(value);
        }
    }

    private static String throwIncomplete() throws IOException { throw new IOException("incomplete_request"); }

    private JSONObject snapshot() {
        Future<JSONObject> pending;
        try {
            pending = snapshots.submit(this::capture);
        } catch (RejectedExecutionException busy) {
            return error("snapshot_busy");
        }
        try {
            return pending.get(SNAPSHOT_TIMEOUT_MS, TimeUnit.MILLISECONDS);
        } catch (TimeoutException timeout) {
            pending.cancel(true);
            return error("snapshot_timeout");
        } catch (InterruptedException stopped) {
            pending.cancel(true);
            Thread.currentThread().interrupt();
            return error("service_stopped");
        } catch (ExecutionException failed) {
            Throwable cause = failed.getCause();
            String reason = (cause instanceof SnapshotException || cause instanceof SnapshotEncoding.LimitException) ? cause.getMessage() : "snapshot_failed";
            Log.w(TAG, "snapshot failed: " + reason);
            return error(reason);
        }
    }

    private JSONObject capture() throws Exception {
        long started = SystemClock.elapsedRealtime();
        KeyguardManager keyguard = (KeyguardManager) getSystemService(KEYGUARD_SERVICE);
        if (keyguard == null || keyguard.isKeyguardLocked()) throw new SnapshotException("device_locked");
        DisplayManager displays = (DisplayManager) getSystemService(DISPLAY_SERVICE);
        Display display = displays == null ? null : displays.getDisplay(Display.DEFAULT_DISPLAY);
        if (display == null) throw new SnapshotException("default_display_unavailable");
        Point size = new Point();
        display.getRealSize(size);
        if (size.x <= 0 || size.y <= 0) throw new SnapshotException("invalid_display_size");

        List<AccessibilityWindowInfo> windows;
        if (Build.VERSION.SDK_INT >= 30) {
            SparseArray<List<AccessibilityWindowInfo>> all = getWindowsOnAllDisplays();
            windows = all.get(Display.DEFAULT_DISPLAY);
        } else {
            windows = getWindows();
        }
        if (windows == null || windows.isEmpty()) throw new SnapshotException("no_default_display_windows");
        windows = new ArrayList<>(windows);
        windows.sort(Comparator.comparingInt(AccessibilityWindowInfo::getLayer).reversed());
        SnapshotEncoding.LimitedOutput output = new SnapshotEncoding.LimitedOutput(MAX_XML);
        XmlSerializer xml = Xml.newSerializer();
        xml.setOutput(output, "UTF-8");
        xml.startDocument("UTF-8", true);
        xml.startTag(null, "hierarchy");
        xml.attribute(null, "rotation", Integer.toString(display.getRotation()));
        Walk walk = new Walk(started + SNAPSHOT_TIMEOUT_MS, xml, size);
        int roots = 0;
        try {
            for (AccessibilityWindowInfo window : windows) {
                walk.check();
                if (Build.VERSION.SDK_INT >= 30 && window.getDisplayId() != Display.DEFAULT_DISPLAY) {
                    throw new SnapshotException("non_default_display");
                }
                AccessibilityNodeInfo root = null;
                try {
                    if (window.isActive()) {
                        root = getRootInActiveWindow();
                        if (root != null && root.getWindowId() != window.getId()) {
                            root.recycle();
                            root = null;
                        }
                    }
                    if (root == null) root = window.getRoot();
                    if (root == null && (window.isActive() || window.getType() == AccessibilityWindowInfo.TYPE_APPLICATION)) {
                        throw new SnapshotException("window_root_unavailable");
                    }
                    if (root != null && root.isVisibleToUser()) {
                        walk.node(root, roots, 0);
                        roots++;
                    } else if (window.isActive()) {
                        throw new SnapshotException("active_root_not_visible");
                    }
                } finally {
                    if (root != null) root.recycle();
                }
            }
        } finally {
            for (AccessibilityWindowInfo window : windows) window.recycle();
        }
        if (roots == 0 || walk.nodes == 0) throw new SnapshotException("no_visible_root");
        walk.check();
        xml.endTag(null, "hierarchy");
        xml.endDocument();
        xml.flush();
        String text = new String(output.toByteArray(), StandardCharsets.UTF_8);
        JSONObject response = new JSONObject();
        response.put("version", CompanionProtocol.VERSION);
        response.put("xml", text);
        response.put("width", size.x);
        response.put("height", size.y);
        response.put("captured_at_ms", System.currentTimeMillis());
        Log.i(TAG, "snapshot elapsed_ms=" + (SystemClock.elapsedRealtime() - started) + " nodes=" + walk.nodes + " xml_bytes=" + output.size());
        return response;
    }

    private final class Walk {
        final long deadline;
        final XmlSerializer xml;
        final Point size;
        int nodes;
        Walk(long deadline, XmlSerializer xml, Point size) { this.deadline = deadline; this.xml = xml; this.size = size; }
        void check() throws SnapshotException {
            if (!running || Thread.currentThread().isInterrupted()) throw new SnapshotException("service_stopped");
            if (SystemClock.elapsedRealtime() >= deadline) throw new SnapshotException("snapshot_timeout");
        }
        void node(AccessibilityNodeInfo node, int index, int depth) throws Exception {
            check();
            if (!node.isVisibleToUser()) return;
            if (depth >= MAX_DEPTH) throw new SnapshotException("tree_depth_limit");
            Rect bounds = new Rect();
            node.getBoundsInScreen(bounds);
            // Bounds remain in the real display's coordinate space, including insets.
            if (!bounds.intersect(0, 0, size.x, size.y)) return;
            if (++nodes > MAX_NODES) throw new SnapshotException("tree_node_limit");
            xml.startTag(null, "node");
            attr("index", Integer.toString(index));
            attr("text", node.isPassword() ? "" : node.getText());
            attr("resource-id", node.getViewIdResourceName());
            attr("class", node.getClassName());
            attr("package", node.getPackageName());
            attr("content-desc", node.isPassword() ? "" : node.getContentDescription());
            attr("checkable", Boolean.toString(node.isCheckable()));
            attr("checked", Boolean.toString(node.isChecked()));
            attr("clickable", Boolean.toString(node.isClickable()));
            attr("enabled", Boolean.toString(node.isEnabled()));
            attr("focusable", Boolean.toString(node.isFocusable()));
            attr("focused", Boolean.toString(node.isFocused()));
            attr("scrollable", Boolean.toString(node.isScrollable()));
            attr("long-clickable", Boolean.toString(node.isLongClickable()));
            attr("password", Boolean.toString(node.isPassword()));
            attr("selected", Boolean.toString(node.isSelected()));
            attr("editable", Boolean.toString(node.isEditable()));
            attr("bounds", "[" + bounds.left + "," + bounds.top + "][" + bounds.right + "," + bounds.bottom + "]");
            int children = node.getChildCount();
            for (int i = 0; i < children; i++) {
                check();
                AccessibilityNodeInfo child = node.getChild(i);
                if (child != null) try { node(child, i, depth + 1); } finally { child.recycle(); }
            }
            xml.endTag(null, "node");
        }
        void attr(String name, CharSequence value) throws IOException {
            xml.attribute(null, name, SnapshotEncoding.xmlText(value));
        }
    }

    // Reply directly on the client handler: no snapshot queue, keyguard check or window Binder call.
    private static JSONObject pong() {
        JSONObject response = new JSONObject();
        try { response.put("version", CompanionProtocol.VERSION); response.put("command", CompanionProtocol.PONG); }
        catch (Exception impossible) { throw new IllegalStateException(impossible); }
        return response;
    }

    private static JSONObject error(String reason) {
        JSONObject response = new JSONObject();
        try { response.put("version", CompanionProtocol.VERSION); response.put("error", reason); }
        catch (Exception impossible) { throw new IllegalStateException(impossible); }
        return response;
    }
    private static final class SnapshotException extends Exception {
        SnapshotException(String message) { super(message); }
    }
    private static void close(LocalSocket socket) {
        try { socket.shutdownInput(); } catch (IOException ignored) { }
        try { socket.shutdownOutput(); } catch (IOException ignored) { }
        try { socket.close(); } catch (IOException ignored) { }
    }
    private void stopServer() {
        running = false;
        LocalServerSocket listener = server;
        server = null;
        if (listener != null) {
            try { Os.shutdown(listener.getFileDescriptor(), OsConstants.SHUT_RDWR); } catch (Exception ignored) { }
            try { listener.close(); } catch (IOException ignored) { }
        }
        for (LocalSocket socket : sockets) close(socket);
        sockets.clear();
        if (connections != null) connections.shutdownNow();
        if (snapshots != null) {
            for (Runnable pending : snapshots.shutdownNow()) if (pending instanceof Future<?>) ((Future<?>) pending).cancel(true);
        }
        if (acceptThread != null) acceptThread.interrupt();
        acceptThread = null;
    }
    @Override public void onAccessibilityEvent(AccessibilityEvent event) { /* Live reads only; never cache content. */ }
    @Override public void onInterrupt() { Log.i(TAG, "accessibility feedback interrupted"); }
    @Override public boolean onUnbind(Intent intent) { stopServer(); return super.onUnbind(intent); }
    @Override public void onDestroy() { stopServer(); super.onDestroy(); }
}
