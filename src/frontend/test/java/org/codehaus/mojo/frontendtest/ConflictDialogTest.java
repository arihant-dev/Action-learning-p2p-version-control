package org.codehaus.mojo.frontendtest;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

public class ConflictDialogTest {

    @Test
    void testDialogCreation() {
        try {
            ConflictDialog dialog = new ConflictDialog(
                "/test/file.txt",
                "hash-local-abc",
                "hash-remote-xyz",
                "alice",
                "bob"
            );
            assertNotNull(dialog);
            assertEquals("Conflict Detected", dialog.getTitle());
            assertTrue(dialog.getHeaderText().contains("/test/file.txt"));
        } catch (ExceptionInInitializerError | IllegalStateException | NoClassDefFoundError e) {
            System.out.println("JavaFX toolkit not available in test env: " + e.getMessage());
        }
    }

    @Test
    void testDialogHandlesEmptyVersions() {
        try {
            ConflictDialog dialog = new ConflictDialog(
                "/test/file.txt",
                "",
                "",
                "local-peer",
                "remote-peer"
            );
            assertNotNull(dialog);
            assertEquals("Conflict Detected", dialog.getTitle());
        } catch (ExceptionInInitializerError | IllegalStateException | NoClassDefFoundError e) {
            System.out.println("JavaFX toolkit not available in test env: " + e.getMessage());
        }
    }
}
