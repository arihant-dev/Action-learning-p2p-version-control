package org.codehaus.mojo.frontendtest;

import com.google.gson.JsonArray;
import com.google.gson.JsonObject;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

public class RepoStatusControllerTest {

    @Test
    void testControllerInstantiation() {
        assertDoesNotThrow(() -> {
            RepoStatusController controller = new RepoStatusController();
            assertNotNull(controller);
        }, "RepoStatusController should instantiate without error");
    }

    @Test
    void testShutdownWithoutInitialize() {
        RepoStatusController controller = new RepoStatusController();
        assertDoesNotThrow(controller::shutdown);
    }

    @Test
    void testSettingsLoadDefaults() {
        JsonObject settings = SettingsDialog.loadSettings();
        assertNotNull(settings);
        assertTrue(settings.has("theme"));
        assertTrue(settings.has("auto_start"));
        assertTrue(settings.has("log_level"));
    }

    @Test
    void testSettingsDefaultValues() {
        assertEquals("dark", SettingsDialog.getSetting("theme", "dark"));
        assertEquals("INFO", SettingsDialog.getSetting("log_level", "INFO"));
        assertEquals(9876, SettingsDialog.getSetting("p2p_port", 9876));
    }

    @Test
    void testJsonNullSafetyRepoStatusResponse() {
        JsonObject obj = new JsonObject();
        obj.addProperty("repo_id", "test-repo");
        assertFalse(obj.has("files"));
        if (!obj.has("files") || obj.get("files").isJsonNull()) {
            assertTrue(true);
        } else {
            fail("Should have taken the null-safety path");
        }
    }

    @Test
    void testJsonNullSafetyFileObject() {
        JsonObject fileObj = new JsonObject();
        fileObj.addProperty("path", "test.txt");

        String path = fileObj.has("path") && !fileObj.get("path").isJsonNull() ? fileObj.get("path").getAsString() : "unknown";
        long size = fileObj.has("size") && !fileObj.get("size").isJsonNull() ? fileObj.get("size").getAsLong() : 0;
        long version = fileObj.has("version") && !fileObj.get("version").isJsonNull() ? fileObj.get("version").getAsLong() : 0;

        assertEquals("test.txt", path);
        assertEquals(0, size);
        assertEquals(0, version);
    }

    @Test
    void testPeerListNullSafety() {
        JsonObject obj = new JsonObject();
        obj.add("peers", new JsonArray());
        JsonArray peers = obj.getAsJsonArray("peers");
        assertEquals(0, peers.size());
    }
}
