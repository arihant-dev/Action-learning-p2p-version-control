package org.codehaus.mojo.frontendtest;

import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import javafx.animation.KeyFrame;
import javafx.animation.Timeline;
import javafx.application.Platform;
import javafx.fxml.FXML;
import javafx.fxml.FXMLLoader;
import javafx.geometry.Insets;
import javafx.scene.Scene;
import javafx.scene.control.*;
import javafx.scene.layout.GridPane;
import javafx.scene.layout.VBox;
import javafx.stage.Stage;
import javafx.stage.WindowEvent;
import javafx.util.Duration;

import java.util.HashMap;
import java.util.Map;

public class RepositoryListController {
    @FXML
    private ListView<String> repoListView;
    @FXML
    private VBox rootContainer;
    @FXML
    private Button themeToggleButton;
    @FXML
    private MenuBar menuBar;
    private boolean isDarkMode = true;

    private Timeline pollTimeline;
    private final IpcBridge.MessageListener repoListListener = this::handleRepoListResponse;

    public void initialize() {
        IpcBridge bridge = IpcBridge.getInstance();

        bridge.registerListener("repo_list_response", repoListListener);

        pollTimeline = new Timeline(new KeyFrame(Duration.seconds(1.5), event -> {
            bridge.send("repo_list_request", new Object());
        }));
        pollTimeline.setCycleCount(Timeline.INDEFINITE);
        pollTimeline.play();

        bridge.send("repo_list_request", new Object());

        repoListView.sceneProperty().addListener((obs, oldScene, newScene) -> {
            if (newScene != null) {
                newScene.windowProperty().addListener((obs2, oldWindow, newWindow) -> {
                    if (newWindow != null) {
                        newWindow.addEventFilter(WindowEvent.WINDOW_CLOSE_REQUEST, event -> shutdown());
                    }
                });
            }
        });

        String savedTheme = SettingsDialog.getSetting("theme", "dark");
        isDarkMode = "dark".equals(savedTheme);
        rootContainer.getStylesheets().addAll(
                getClass().getResource("styles.css").toExternalForm(),
                getClass().getResource(savedTheme + ".css").toExternalForm()
        );
        themeToggleButton.setText(isDarkMode ? "lightmode" : "darkmode");
    }

    public void shutdown() {
        if (pollTimeline != null) {
            pollTimeline.stop();
        }
        IpcBridge.getInstance().removeListener("repo_list_response", repoListListener);
    }

    private void handleRepoListResponse(JsonElement payload) {
        if (payload == null || !payload.isJsonObject()) return;

        JsonObject obj = payload.getAsJsonObject();
        if (!obj.has("repos") || obj.get("repos").isJsonNull()) return;

        JsonArray repos = obj.getAsJsonArray("repos");

        int selectedIndex = repoListView.getSelectionModel().getSelectedIndex();

        repoListView.getItems().clear();
        for (JsonElement repoEl : repos) {
            if (repoEl != null && repoEl.isJsonObject()) {
                JsonObject repoObj = repoEl.getAsJsonObject();
                if (repoObj.has("id") && !repoObj.get("id").isJsonNull()) {
                    repoListView.getItems().add(repoObj.get("id").getAsString());
                }
            }
        }

        if (selectedIndex >= 0 && selectedIndex < repoListView.getItems().size()) {
            repoListView.getSelectionModel().select(selectedIndex);
        }
    }

    @FXML
    private void handleThemeToggle() {
        rootContainer.getStylesheets().removeIf(sheet ->
                sheet.contains("dark.css") || sheet.contains("light.css")
        );

        if (isDarkMode) {
            rootContainer.getStylesheets().add(getClass().getResource("light.css").toExternalForm());
            themeToggleButton.setText("darkmode");
            isDarkMode = false;
        } else {
            rootContainer.getStylesheets().add(getClass().getResource("dark.css").toExternalForm());
            themeToggleButton.setText("lightmode");
            isDarkMode = true;
        }
    }

    @FXML
    protected void handleSettings() {
        Stage stage = (Stage) rootContainer.getScene().getWindow();
        SettingsDialog.show(stage);
    }

    @FXML
    protected void handleQuit() {
        shutdown();
        Platform.exit();
    }

    @FXML
    protected void handleRepoClick() {
        String selected = repoListView.getSelectionModel().getSelectedItem();
        if (selected == null) return;

        try {
            FXMLLoader fxmlLoader = new FXMLLoader(HelloApplication.class.getResource("RepoStatusView.fxml"));
            Scene scene = new Scene(fxmlLoader.load(), 600, 450);

            String activeThemeFile = isDarkMode ? "dark.css" : "light.css";
            scene.getStylesheets().addAll(
                    HelloApplication.class.getResource("styles.css").toExternalForm(),
                    getClass().getResource(activeThemeFile).toExternalForm()
            );

            RepoStatusController controller = fxmlLoader.getController();
            controller.setRepoId(selected);

            Stage stage = new Stage();
            stage.setTitle("Repo Status: " + selected);
            stage.setScene(scene);
            stage.show();
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    @FXML
    protected void handleAddRepo() {
        showAddRepoDialog(false);
    }

    @FXML
    protected void handleShareRepo() {
        String selected = repoListView.getSelectionModel().getSelectedItem();
        if (selected == null) {
            showAlert("No Repository Selected", "Please select a repository to share first.");
            return;
        }

        JsonObject payload = new JsonObject();
        payload.addProperty("repo_id", selected);
        IpcBridge.getInstance().send("share_repository", payload);

        Alert alert = new Alert(Alert.AlertType.INFORMATION,
            "Repository " + selected + " is being shared with all connected peers.", ButtonType.OK);
        alert.setTitle("Share Repository");
        alert.setHeaderText(null);
        alert.showAndWait();
    }

    @FXML
    protected void handleJoinRepo() {
        showAddRepoDialog(true);
    }

    private void showAddRepoDialog(boolean isJoin) {
        Dialog<Map<String, String>> dialog = new Dialog<>();
        dialog.setTitle(isJoin ? "Join Existing Repository" : "Add New Repository");
        dialog.setHeaderText(isJoin ? "Join a repository shared by a peer" : "Create and track a local repository");

        String activeThemeFile = isDarkMode ? "dark.css" : "light.css";
        dialog.getDialogPane().getStylesheets().addAll(
                HelloApplication.class.getResource("styles.css").toExternalForm(),
                getClass().getResource(activeThemeFile).toExternalForm()
        );
        dialog.getDialogPane().getStyleClass().add("root");

        ButtonType actionButtonType = new ButtonType(isJoin ? "Join" : "Create", ButtonBar.ButtonData.OK_DONE);
        dialog.getDialogPane().getButtonTypes().addAll(actionButtonType, ButtonType.CANCEL);

        GridPane grid = new GridPane();
        grid.setHgap(10);
        grid.setVgap(10);
        grid.setPadding(new Insets(20, 150, 10, 10));

        TextField repoId = new TextField();
        repoId.setPromptText("e.g. project-alpha");
        TextField path = new TextField();
        path.setPromptText("e.g. /path/to/local/folder");

        Button browseBtn = new Button("Browse...");
        browseBtn.setOnAction(e -> {
            javafx.stage.DirectoryChooser dirChooser = new javafx.stage.DirectoryChooser();
            dirChooser.setTitle("Select Repository Folder");
            java.io.File selectedDir = dirChooser.showDialog(dialog.getOwner());
            if (selectedDir != null) {
                path.setText(selectedDir.getAbsolutePath());
            }
        });

        grid.add(new Label("Repo ID:"), 0, 0);
        grid.add(repoId, 1, 0);
        grid.add(new Label("Local Path:"), 0, 1);
        grid.add(path, 1, 1);
        grid.add(browseBtn, 2, 1);

        dialog.getDialogPane().setContent(grid);

        Platform.runLater(repoId::requestFocus);

        dialog.setResultConverter(dialogButton -> {
            if (dialogButton == actionButtonType) {
                Map<String, String> map = new HashMap<>();
                map.put("repo_id", repoId.getText().trim());
                map.put("path", path.getText().trim());
                return map;
            }
            return null;
        });

        dialog.showAndWait().ifPresent(result -> {
            String repoIdValue = result.get("repo_id");
            String pathValue = result.get("path");

            if (repoIdValue.isEmpty()) {
                showAlert("Validation Error", "Repository ID cannot be empty.");
                return;
            }
            if (pathValue.isEmpty()) {
                showAlert("Validation Error", "Local path cannot be empty.");
                return;
            }
            if (!isJoin) {
                java.io.File pathDir = new java.io.File(pathValue);
                if (!pathDir.exists()) {
                    showAlert("Validation Error", "The specified path does not exist: " + pathValue);
                    return;
                }
                if (!pathDir.isDirectory()) {
                    showAlert("Validation Error", "The specified path is not a directory: " + pathValue);
                    return;
                }
            }

            if (isJoin) {
                JsonObject payload = new JsonObject();
                payload.addProperty("repo_id", repoIdValue);
                payload.addProperty("path", pathValue);
                IpcBridge.getInstance().send("join_repository", payload);
            } else {
                IpcBridge.getInstance().send("add_repository", result);
            }
            IpcBridge.getInstance().send("repo_list_request", new Object());
        });
    }

    private void showAlert(String title, String message) {
        Alert alert = new Alert(Alert.AlertType.WARNING, message, ButtonType.OK);
        alert.setTitle(title);
        alert.setHeaderText(null);
        alert.showAndWait();
    }

    @FXML
    protected void handleDeleteRepo() {
        String selected = repoListView.getSelectionModel().getSelectedItem();
        if (selected == null) return;

        Alert confirmation = new Alert(Alert.AlertType.CONFIRMATION, "Are you sure you want to stop tracking and delete the sync record for: " + selected + "?", ButtonType.YES, ButtonType.NO);
        confirmation.setTitle("Untrack Repository");
        confirmation.setHeaderText(null);
        confirmation.showAndWait().ifPresent(response -> {
            if (response == ButtonType.YES) {
                Map<String, String> payload = new HashMap<>();
                payload.put("repo_id", selected);
                IpcBridge.getInstance().send("remove_repository", payload);
                IpcBridge.getInstance().send("repo_list_request", new Object());
            }
        });
    }
}
