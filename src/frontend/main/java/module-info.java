module org.codehaus.mojo.frontendtest {
    requires javafx.controls;
    requires javafx.fxml;

    requires org.controlsfx.controls;
    requires org.kordamp.ikonli.javafx;
    requires javafx.graphics;
    requires com.google.gson;

    opens org.codehaus.mojo.frontendtest to javafx.fxml, com.google.gson;
    exports org.codehaus.mojo.frontendtest;
}