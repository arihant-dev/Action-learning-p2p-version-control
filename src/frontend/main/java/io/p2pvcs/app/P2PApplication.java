package io.p2pvcs.app;

import javafx.application.Application;
import javafx.application.Platform;
import javafx.concurrent.Task;
import javafx.fxml.FXMLLoader;
import javafx.scene.Scene;
import javafx.scene.control.Label;
import javafx.scene.control.ProgressIndicator;
import javafx.scene.layout.StackPane;
import javafx.scene.layout.VBox;
import javafx.stage.Stage;

import java.io.IOException;

public class P2PApplication extends Application {
    @Override
    public void start(Stage stage) throws IOException {
        StackPane splash = new StackPane();
        splash.setStyle("-fx-background-color: -theme-bg;");
        splash.setPrefSize(360, 550);

        VBox splashContent = new VBox(15);
        splashContent.setAlignment(javafx.geometry.Pos.CENTER);

        Label titleLabel = new Label("P2P Version Control");
        titleLabel.setStyle("-fx-font-size: 20px; -fx-font-weight: bold; -fx-text-fill: -theme-text-main;");

        Label statusLabel = new Label("Connecting to sync daemon...");
        statusLabel.setStyle("-fx-text-fill: -theme-text-muted;");

        ProgressIndicator progress = new ProgressIndicator();
        progress.setPrefSize(40, 40);

        splashContent.getChildren().addAll(titleLabel, progress, statusLabel);
        splash.getChildren().add(splashContent);

        Scene splashScene = new Scene(splash, 360, 550);
        splashScene.getStylesheets().add(P2PApplication.class.getResource("styles.css").toExternalForm());
        splashScene.getStylesheets().add(P2PApplication.class.getResource("dark.css").toExternalForm());

        stage.setMinHeight(550);
        stage.setMinWidth(360);
        stage.setTitle("P2P Version Control");
        stage.setScene(splashScene);
        stage.show();

        Task<Void> connectTask = new Task<>() {
            @Override
            protected Void call() throws Exception {
                IpcBridge bridge = IpcBridge.getInstance();
                if (!bridge.connect()) {
                    long deadline = System.currentTimeMillis() + 5000;
                    while (!bridge.isConnected() && System.currentTimeMillis() < deadline) {
                        Thread.sleep(100);
                    }
                    if (!bridge.isConnected()) {
                        throw new RuntimeException("Failed to connect to sync daemon within timeout");
                    }
                }
                return null;
            }
        };

        connectTask.setOnSucceeded(e -> {
            try {
                FXMLLoader fxmlLoader = new FXMLLoader(P2PApplication.class.getResource("repositoryList.fxml"));
                Scene mainScene = new Scene(fxmlLoader.load(), 360, 550);
                mainScene.getStylesheets().add(P2PApplication.class.getResource("styles.css").toExternalForm());

                String theme = SettingsDialog.getSetting("theme", "dark");
                mainScene.getStylesheets().add(P2PApplication.class.getResource(theme + ".css").toExternalForm());

                stage.setScene(mainScene);
            } catch (IOException ex) {
                showErrorAndExit(stage, "Failed to load main UI: " + ex.getMessage());
            }
        });

        connectTask.setOnFailed(e -> {
            Throwable ex = connectTask.getException();
            String msg = ex != null && ex.getMessage() != null ? ex.getMessage() : "Could not connect to sync daemon";
            showErrorAndExit(stage, "Failed to connect to sync daemon: " + msg);
        });

        Thread connectThread = new Thread(connectTask, "IPC-Connect-Thread");
        connectThread.setDaemon(true);
        connectThread.start();
    }

    private void showErrorAndExit(Stage stage, String message) {
        Platform.runLater(() -> {
            VBox errorBox = new VBox(10);
            errorBox.setAlignment(javafx.geometry.Pos.CENTER);
            errorBox.setPadding(new javafx.geometry.Insets(20));

            Label errorLabel = new Label(message);
            errorLabel.setStyle("-fx-text-fill: -theme-danger; -fx-wrap-text: true;");
            errorLabel.setMaxWidth(320);

            errorBox.getChildren().add(errorLabel);

            Scene errorScene = new Scene(errorBox, 360, 200);
            errorScene.getStylesheets().add(P2PApplication.class.getResource("styles.css").toExternalForm());
            errorScene.getStylesheets().add(P2PApplication.class.getResource("dark.css").toExternalForm());

            stage.setScene(errorScene);
        });
    }

    @Override
    public void stop() {
        IpcBridge.getInstance().disconnect();
    }
}
