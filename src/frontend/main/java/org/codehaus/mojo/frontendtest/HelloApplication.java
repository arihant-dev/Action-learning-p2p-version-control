package org.codehaus.mojo.frontendtest;

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

public class HelloApplication extends Application {
    @Override
    public void start(Stage stage) throws IOException {
        StackPane splash = new StackPane();
        splash.setStyle("-fx-background-color: -theme-bg;");
        splash.setPrefSize(360, 550);

        VBox splashContent = new VBox(20);
        splashContent.setAlignment(javafx.geometry.Pos.CENTER);
        splashContent.setPadding(new javafx.geometry.Insets(40));

        VBox titleContainer = new VBox(10);
        titleContainer.setAlignment(javafx.geometry.Pos.CENTER);
        titleContainer.setStyle("-fx-border-color: -theme-border; -fx-border-width: 1px; -fx-padding: 24px;");

        Label titleLabel = new Label("AXON");
        titleLabel.setStyle("-fx-font-family: 'Hanken Grotesk', sans-serif; -fx-font-size: 54px; -fx-font-weight: 900; -fx-text-fill: -theme-text-main; -fx-letter-spacing: -0.04em;");

        javafx.scene.shape.Line line = new javafx.scene.shape.Line(0, 0, 80, 0);
        line.setStyle("-fx-stroke: -theme-border; -fx-stroke-width: 2px;");

        Label subtitleLabel = new Label("PEER-TO-PEER VERSION CONTROL");
        subtitleLabel.setStyle("-fx-font-family: 'Hanken Grotesk', sans-serif; -fx-font-size: 11px; -fx-font-weight: 800; -fx-text-fill: -theme-text-muted; -fx-letter-spacing: 0.1em;");

        titleContainer.getChildren().addAll(titleLabel, line, subtitleLabel);

        VBox statusContainer = new VBox(12);
        statusContainer.setAlignment(javafx.geometry.Pos.CENTER);

        Label statusLabel = new Label("INITIALIZING MESH...");
        statusLabel.setStyle("-fx-font-family: 'JetBrains Mono', monospace; -fx-font-size: 12px; -fx-font-weight: bold; -fx-text-fill: -theme-danger;");

        ProgressIndicator progress = new ProgressIndicator();
        progress.setPrefSize(30, 30);
        progress.setStyle("-fx-progress-color: -theme-danger;");

        statusContainer.getChildren().addAll(progress, statusLabel);

        splashContent.getChildren().addAll(titleContainer, statusContainer);
        splash.getChildren().add(splashContent);

        Scene splashScene = new Scene(splash, 360, 550);
        splashScene.getStylesheets().add(HelloApplication.class.getResource("styles.css").toExternalForm());
        splashScene.getStylesheets().add(HelloApplication.class.getResource("dark.css").toExternalForm());

        stage.setMinHeight(550);
        stage.setMinWidth(360);
        stage.setTitle("P2P Version Control");
        stage.setScene(splashScene);
        stage.show();

        Task<Void> connectTask = new Task<>() {
            @Override
            protected Void call() {
                IpcBridge.getInstance().connect();
                return null;
            }
        };

        connectTask.setOnSucceeded(e -> {
            try {
                FXMLLoader fxmlLoader = new FXMLLoader(HelloApplication.class.getResource("repositoryList.fxml"));
                Scene mainScene = new Scene(fxmlLoader.load(), 360, 550);
                mainScene.getStylesheets().add(HelloApplication.class.getResource("styles.css").toExternalForm());

                String theme = SettingsDialog.getSetting("theme", "dark");
                mainScene.getStylesheets().add(HelloApplication.class.getResource(theme + ".css").toExternalForm());

                stage.setScene(mainScene);
            } catch (IOException ex) {
                showErrorAndExit(stage, "Failed to load main UI: " + ex.getMessage());
            }
        });

        connectTask.setOnFailed(e -> {
            showErrorAndExit(stage, "Failed to connect to sync daemon: " + connectTask.getException().getMessage());
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
            errorScene.getStylesheets().add(HelloApplication.class.getResource("styles.css").toExternalForm());
            errorScene.getStylesheets().add(HelloApplication.class.getResource("dark.css").toExternalForm());

            stage.setScene(errorScene);
        });
    }

    @Override
    public void stop() {
        IpcBridge.getInstance().disconnect();
    }
}
