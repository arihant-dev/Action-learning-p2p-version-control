package org.codehaus.mojo.frontendtest;

import org.junit.jupiter.api.*;
import static org.junit.jupiter.api.Assertions.*;

class RepositoryListControllerTest {

    @Test
    void testControllerInstantiation() {
        assertDoesNotThrow(() -> {
            RepositoryListController controller = new RepositoryListController();
            assertNotNull(controller);
        }, "RepositoryListController should instantiate without error");
    }

    @Test
    void testShutdown() {
        RepositoryListController controller = new RepositoryListController();
        assertDoesNotThrow(() -> {
            controller.shutdown();
        }, "shutdown() should not throw");
    }

    @Test
    void testShutdownWithoutInitialize() {
        RepositoryListController controller = new RepositoryListController();
        assertDoesNotThrow(controller::shutdown);
    }

    @Test
    void testSettingsPersistence() {
        String theme = SettingsDialog.getSetting("theme", "dark");
        assertNotNull(theme);
        assertTrue(theme.equals("dark") || theme.equals("light"));
    }

    @Test
    void testSettingsIntValue() {
        int port = SettingsDialog.getSetting("p2p_port", 9876);
        assertTrue(port > 0 && port <= 65535, "Port should be in valid range");
    }
}
