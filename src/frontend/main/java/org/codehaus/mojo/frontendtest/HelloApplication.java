package org.codehaus.mojo.frontendtest;

import javafx.application.Application;
import javafx.fxml.FXMLLoader;
import javafx.scene.Scene;
import javafx.stage.Stage;

import java.io.IOException;

public class HelloApplication extends Application {
    @Override
    public void start(Stage stage) throws IOException {
        IpcBridge.getInstance().connect();

        FXMLLoader fxmlLoader = new FXMLLoader(HelloApplication.class.getResource("repositoryList.fxml"));
        Scene scene = new Scene(fxmlLoader.load(), 360, 550);
        scene.getStylesheets().add(HelloApplication.class.getResource("styles.css").toExternalForm());
        stage.setMinHeight(550);
        stage.setMinWidth(360);
        stage.setTitle("P2P Version Control");
        stage.setScene(scene);
        stage.show();
    }

    @Override
    public void stop() {
        IpcBridge.getInstance().disconnect();
    }
}
