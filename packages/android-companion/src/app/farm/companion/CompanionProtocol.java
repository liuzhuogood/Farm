package app.farm.companion;

import java.util.function.Supplier;

/** Protocol dispatch is independent of Android state and the tree worker. */
final class CompanionProtocol {
    static final int VERSION = 2;
    static final String PONG = "pong";

    private CompanionProtocol() { }

    static <T> T dispatch(Object version, Object command, Supplier<T> ping, Supplier<T> snapshot) {
        if (!Integer.valueOf(VERSION).equals(version)) throw new UnsupportedRequest();
        // Liveness must stay available when tree collection is blocked or busy.
        if ("ping".equals(command)) return ping.get();
        if ("snapshot".equals(command)) return snapshot.get();
        throw new UnsupportedRequest();
    }

    static final class UnsupportedRequest extends RuntimeException { }
}
