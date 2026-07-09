package org.codehaus.mojo.frontendtest;

import com.google.gson.JsonObject;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

public class RepoStatusControllerTest {

    @Test
    void testConflictInfoFromJson() {
        JsonObject obj = new JsonObject();
        obj.addProperty("path", "/conflict/file.txt");
        obj.addProperty("local_peer", "alice");
        obj.addProperty("remote_peer", "bob");

        IpcBridge.ConflictInfo info = IpcBridge.parseConflict(obj);
        assertEquals("/conflict/file.txt", info.filePath);
        assertEquals("alice", info.localPeer);
        assertEquals("bob", info.remotePeer);
    }

    @Test
    void testConflictInfoEmptyJson() {
        JsonObject obj = new JsonObject();
        IpcBridge.ConflictInfo info = IpcBridge.parseConflict(obj);
        assertEquals("unknown", info.filePath);
        assertEquals("you", info.localPeer);
        assertEquals("unknown", info.remotePeer);
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
}
