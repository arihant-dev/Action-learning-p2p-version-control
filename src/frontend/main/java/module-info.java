module io.p2pvcs.app {
    requires javafx.controls;
    requires javafx.fxml;

    requires org.kordamp.ikonli.core;
    requires org.kordamp.ikonli.javafx;
    requires javafx.graphics;
    requires com.google.gson;

    opens io.p2pvcs.app to javafx.fxml, com.google.gson;
    exports io.p2pvcs.app;
}