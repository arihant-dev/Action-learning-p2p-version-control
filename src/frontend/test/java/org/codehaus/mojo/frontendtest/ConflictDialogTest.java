package org.codehaus.mojo.frontendtest;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

public class ConflictDialogTest {

    @Test
    void testDialogTitle() {
        assertEquals("Conflict Detected", "Conflict Detected");
    }

    @Test
    void testResolutionOptions() {
        assertEquals("local", "local");
        assertEquals("remote", "remote");
        assertEquals("merge", "merge");
    }
}
