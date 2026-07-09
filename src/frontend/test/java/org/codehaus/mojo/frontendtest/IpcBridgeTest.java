package org.codehaus.mojo.frontendtest;

import org.junit.jupiter.api.*;
import static org.junit.jupiter.api.Assertions.*;

class IpcBridgeTest {

    @BeforeEach
    void setUp() {
        // Ensure clean state by disconnecting before each test
        IpcBridge bridge = IpcBridge.getInstance();
        bridge.disconnect();
    }

    @AfterEach
    void tearDown() {
        IpcBridge bridge = IpcBridge.getInstance();
        bridge.disconnect();
    }

    @Test
    void testSingleton() {
        IpcBridge instance1 = IpcBridge.getInstance();
        IpcBridge instance2 = IpcBridge.getInstance();
        assertSame(instance1, instance2, "getInstance() should return the same instance");
    }

    @Test
    void testConnectDisconnect() {
        IpcBridge bridge = IpcBridge.getInstance();
        assertDoesNotThrow(() -> bridge.disconnect(), "disconnect() should not throw when not connected");
    }

    @Test
    void testDisconnectMultipleTimes() {
        IpcBridge bridge = IpcBridge.getInstance();
        assertDoesNotThrow(() -> {
            bridge.disconnect();
            bridge.disconnect();
            bridge.disconnect();
        }, "Multiple disconnect() calls should not throw");
    }

    @Test
    void testSendWithoutConnect() {
        IpcBridge bridge = IpcBridge.getInstance();
        assertDoesNotThrow(() -> {
            bridge.send("ping", "test");
        }, "send() should not throw when not connected");
    }

    @Test
    void testRegisterListener() {
        IpcBridge bridge = IpcBridge.getInstance();
        IpcBridge.MessageListener listener = payload -> {};
        assertDoesNotThrow(() -> {
            bridge.registerListener("test_type", listener);
        }, "registerListener() should not throw");
    }

    @Test
    void testRegisterAndRemoveListener() {
        IpcBridge bridge = IpcBridge.getInstance();
        IpcBridge.MessageListener listener = payload -> {};
        bridge.registerListener("test_type", listener);
        assertDoesNotThrow(() -> {
            bridge.removeListener("test_type", listener);
        }, "removeListener() should not throw");
    }

    @Test
    void testRemoveNonExistentListener() {
        IpcBridge bridge = IpcBridge.getInstance();
        IpcBridge.MessageListener listener = payload -> {};
        assertDoesNotThrow(() -> {
            bridge.removeListener("nonexistent_type", listener);
        }, "removeListener() for non-existent type should not throw");
    }
}
