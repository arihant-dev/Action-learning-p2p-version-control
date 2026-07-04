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

        rootContainer.getStylesheets().addAll(
                getClass().getResource("styles.css").toExternalForm(),
                getClass().getResource("dark.css").toExternalForm()
        );
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
            if (repoEl.isJsonObject()) {
                JsonObject repoObj = repoEl.getAsJsonObject();
                if (repoObj.has("id")) {
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
            Alert alert = new Alert(Alert.AlertType.WARNING, "Please select a repository to share first.", ButtonType.OK);
            alert.showAndWait();
            return;
        }
        Alert alert = new Alert(Alert.AlertType.INFORMATION, "Repository " + selected + " is automatically discoverable on your local network for other peers!", ButtonType.OK);
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

        grid.add(new Label("Repo ID:"), 0, 0);
        grid.add(repoId, 1, 0);
        grid.add(new Label("Local Path:"), 0, 1);
        grid.add(path, 1, 1);

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
            if (!result.get("repo_id").isEmpty() && !result.get("path").isEmpty()) {
                IpcBridge.getInstance().send("add_repository", result);
                IpcBridge.getInstance().send("repo_list_request", new Object());
            }
        });
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
