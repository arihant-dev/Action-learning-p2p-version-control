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
        // Call shutdown without initialize - should handle gracefully
        assertDoesNotThrow(controller::shutdown);
    }
}
