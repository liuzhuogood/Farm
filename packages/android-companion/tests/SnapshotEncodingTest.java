package app.farm.companion;

import java.nio.charset.StandardCharsets;
import java.io.ByteArrayInputStream;
import javax.xml.parsers.DocumentBuilderFactory;

/** Host checks on content and byte limits; never connects to Android devices. */
public final class SnapshotEncodingTest {
    public static void main(String[] args) throws Exception {
        String valid = "设置 😀\t\n\r";
        String invalid = "\u0000\u0001\u000B\u001F\uD800" + valid + "\uDC00\uFFFE\uFFFF";
        String actual = SnapshotEncoding.xmlText(invalid);
        if (!valid.equals(actual)) throw new AssertionError("Unicode sanitizer corrupted valid text or kept invalid code points");
        if (!"".equals(SnapshotEncoding.xmlText(null))) throw new AssertionError("null input");
        String document = "<node>" + actual + "</node>";
        DocumentBuilderFactory factory = DocumentBuilderFactory.newInstance();
        factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);
        factory.newDocumentBuilder().parse(new ByteArrayInputStream(document.getBytes(StandardCharsets.UTF_8)));

        // A byte budget must count UTF-8 bytes rather than Java UTF-16 characters.
        byte[] chinese = "中文".getBytes(StandardCharsets.UTF_8);
        SnapshotEncoding.LimitedOutput limited = new SnapshotEncoding.LimitedOutput(chinese.length);
        limited.write(chinese, 0, chinese.length);
        try { limited.write(0x20); throw new AssertionError("accepted XML beyond the byte limit"); }
        catch (SnapshotEncoding.LimitException expected) {
            if (!"xml_size_limit".equals(expected.getMessage())) throw new AssertionError("lost actionable limit reason");
        }
        if (limited.size() != chinese.length) throw new AssertionError("failed write mutated the response");
        try { limited.write(new byte[8], 0, 8); throw new AssertionError("accepted oversized response chunk"); }
        catch (SnapshotEncoding.LimitException expected) { }
        System.out.println("XML Unicode and strict byte-budget host checks passed");
    }
}
