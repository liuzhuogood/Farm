package app.farm.companion;

import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

public final class CompanionProtocolTest {
    public static void main(String[] args) throws Exception {
        CountDownLatch release = new CountDownLatch(1);
        CountDownLatch occupied = new CountDownLatch(1);
        ThreadPoolExecutor worker = new ThreadPoolExecutor(1, 1, 0, TimeUnit.MILLISECONDS,
            new ArrayBlockingQueue<Runnable>(1));
        try {
            worker.execute(() -> {
                occupied.countDown();
                try { release.await(); } catch (InterruptedException stopped) { Thread.currentThread().interrupt(); }
            });
            if (!occupied.await(2, TimeUnit.SECONDS)) throw new AssertionError("tree worker did not start");
            worker.execute(() -> { }); // Occupy its one permitted pending slot too.
            AtomicInteger snapshotCalls = new AtomicInteger();
            String result = CompanionProtocol.dispatch(Integer.valueOf(2), "ping", () -> CompanionProtocol.PONG, () -> {
                snapshotCalls.incrementAndGet();
                worker.execute(() -> { }); // This is rejected while the tree worker and queue are full.
                return "unexpected snapshot";
            });
            if (!"pong".equals(result) || snapshotCalls.get() != 0) throw new AssertionError("ping entered tree collection");
            if (CompanionProtocol.VERSION != 2) throw new AssertionError("unexpected current protocol");
            for (Object oldVersion : new Object[] {Integer.valueOf(1), "2", Double.valueOf(2), null}) {
                try {
                    CompanionProtocol.dispatch(oldVersion, "ping", () -> "unexpected", () -> "unexpected");
                    throw new AssertionError("accepted unsupported protocol version");
                } catch (CompanionProtocol.UnsupportedRequest expected) { }
            }
            for (Object unknown : new Object[] {"pong", "unknown", Integer.valueOf(1), null}) {
                try {
                    CompanionProtocol.dispatch(Integer.valueOf(2), unknown, () -> "unexpected", () -> "unexpected");
                    throw new AssertionError("accepted unsupported command");
                } catch (CompanionProtocol.UnsupportedRequest expected) { }
            }
            AtomicInteger reads = new AtomicInteger();
            String snapshot = CompanionProtocol.dispatch(Integer.valueOf(2), "snapshot", () -> "unexpected", () -> {
                reads.incrementAndGet();
                return "tree";
            });
            if (!"tree".equals(snapshot) || reads.get() != 1) throw new AssertionError("snapshot no longer reaches its own collector");
        } finally {
            release.countDown();
            worker.shutdownNow();
            if (!worker.awaitTermination(2, TimeUnit.SECONDS)) throw new AssertionError("test worker did not stop");
        }
        System.out.println("Protocol v2 rejects v1; ping bypasses an occupied tree worker and full queue");
    }
}
