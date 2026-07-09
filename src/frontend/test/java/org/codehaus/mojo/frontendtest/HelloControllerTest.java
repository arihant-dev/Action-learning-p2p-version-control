package org.codehaus.mojo.frontendtest;

import org.junit.jupiter.api.*;
import static org.junit.jupiter.api.Assertions.*;

class HelloControllerTest {

    @Test
    void testControllerInstantiation() {
        assertDoesNotThrow(() -> {
            HelloController controller = new HelloController();
            assertNotNull(controller);
        }, "HelloController should instantiate without error");
    }

    @Test
    void testInitialize() {
        HelloController controller = new HelloController();
        assertDoesNotThrow(controller::initialize);
    }

    @Test
    void testOnHelloButtonClick() {
        HelloController controller = new HelloController();
        assertDoesNotThrow(controller::onHelloButtonClick);
    }
}
