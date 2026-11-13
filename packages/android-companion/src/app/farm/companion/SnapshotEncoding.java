package app.farm.companion;

import java.io.ByteArrayOutputStream;

/** Android-independent XML 1.0 text and byte-budget handling. */
final class SnapshotEncoding {
    private SnapshotEncoding() { }

    static String xmlText(CharSequence input) {
        if (input == null) return "";
        String value = input.toString();
        StringBuilder clean = new StringBuilder(value.length());
        for (int offset = 0; offset < value.length();) {
            int code = value.codePointAt(offset);
            if (code == 9 || code == 10 || code == 13 || (code >= 32 && code <= 0xD7FF) ||
                (code >= 0xE000 && code <= 0xFFFD) || (code >= 0x10000 && code <= 0x10FFFF)) clean.appendCodePoint(code);
            offset += Character.charCount(code);
        }
        return clean.toString();
    }

    static final class LimitException extends RuntimeException {
        LimitException(String message) { super(message); }
    }

    static final class LimitedOutput extends ByteArrayOutputStream {
        final int limit;
        LimitedOutput(int limit) { this.limit = limit; }
        private void require(int size) {
            if ((long) count + size > limit) throw new LimitException("xml_size_limit");
        }
        @Override public synchronized void write(int value) { require(1); super.write(value); }
        @Override public synchronized void write(byte[] bytes, int offset, int length) {
            if (bytes == null) throw new NullPointerException("bytes");
            if (offset < 0 || length < 0 || offset > bytes.length - length) throw new IndexOutOfBoundsException();
            require(length);
            super.write(bytes, offset, length);
        }
    }
}
