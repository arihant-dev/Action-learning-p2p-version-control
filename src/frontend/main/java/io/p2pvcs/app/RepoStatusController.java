package io.p2pvcs.app;

import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import javafx.animation.KeyFrame;
import javafx.animation.Timeline;
import javafx.application.Platform;
import javafx.fxml.FXML;
import javafx.scene.control.Alert;
import javafx.scene.control.ButtonType;
import javafx.scene.control.Label;
import javafx.scene.control.ListView;
import javafx.scene.control.TextArea;
import javafx.stage.WindowEvent;
import javafx.util.Duration;

import java.text.SimpleDateFormat;
import java.util.Date;

public class RepoStatusController {
    @FXML
    private Label repoNameLabel;

    @FXML
    private ListView<String> filesListView;

    @FXML
    private TextArea changesTextArea;

    @FXML
    private Label syncStatusLabel;

    @FXML
    private Label peersLabel;

    @FXML
    private Label onlineStatusLabel;

    private String repoId;
    private Timeline pollTimeline;
    private final SimpleDateFormat dateFormat = new SimpleDateFormat("HH:mm:ss");
    private boolean connectedLogged = false;
    private final IpcBridge.MessageListener repoStatusListener = this::handleRepoStatusResponse;
    private final IpcBridge.MessageListener peerListListener = this::handlePeerListUpdate;
    private final IpcBridge.MessageListener conflictListener = this::handleConflictDetected;
    private final IpcBridge.MessageListener syncFromPeerListener = this::handleSyncFromPeer;
    private final IpcBridge.MessageListener transferCompleteListener = this::handleFileTransferComplete;

    public void setRepoId(String repoId) {
        this.repoId = repoId;
        repoNameLabel.setText("Repository: " + repoId);

        pollStatus();
    }

    public void initialize() {
        IpcBridge bridge = IpcBridge.getInstance();
        bridge.registerListener("repo_status_response", repoStatusListener);
        bridge.registerListener("peer_list_update", peerListListener);
        bridge.registerListener("conflict_detected", conflictListener);
        bridge.registerListener("sync_from_peer", syncFromPeerListener);
        bridge.registerListener("file_transfer_complete", transferCompleteListener);

        pollTimeline = new Timeline(new KeyFrame(Duration.seconds(2.0), event -> pollStatus()));
        pollTimeline.setCycleCount(Timeline.INDEFINITE);
        pollTimeline.play();

        bridge.send("peer_list_request", new Object());

        logToConsole("Status window initialized. Connecting to sync daemon...");

        repoNameLabel.sceneProperty().addListener((obs, oldScene, newScene) -> {
            if (newScene != null) {
                newScene.windowProperty().addListener((obs2, oldWindow, newWindow) -> {
                    if (newWindow != null) {
                        newWindow.addEventFilter(WindowEvent.WINDOW_CLOSE_REQUEST, event -> shutdown());
                    }
                });
            }
        });
    }

    private void pollStatus() {
        if (repoId == null) return;

        JsonObject payload = new JsonObject();
        payload.addProperty("repo_id", repoId);
        IpcBridge.getInstance().send("repo_status_request", payload);
        IpcBridge.getInstance().send("peer_list_request", new Object());
    }

    private void handleRepoStatusResponse(JsonElement payload) {
        if (payload == null || !payload.isJsonObject()) return;

        JsonObject obj = payload.getAsJsonObject();
        if (!obj.has("repo_id") || obj.get("repo_id").isJsonNull() || !obj.get("repo_id").getAsString().equals(repoId)) {
            return;
        }

        if (!connectedLogged) {
            connectedLogged = true;
            logToConsole("Connected to sync daemon. Active.");
        }

        if (!obj.has("files") || obj.get("files").isJsonNull()) return;
        JsonArray files = obj.getAsJsonArray("files");

        int selectedIndex = filesListView.getSelectionModel().getSelectedIndex();
        filesListView.getItems().clear();

        for (JsonElement fileEl : files) {
            if (fileEl != null && fileEl.isJsonObject()) {
                JsonObject fileObj = fileEl.getAsJsonObject();
                String path = fileObj.has("path") && !fileObj.get("path").isJsonNull() ? fileObj.get("path").getAsString() : "unknown";
                long size = fileObj.has("size") && !fileObj.get("size").isJsonNull() ? fileObj.get("size").getAsLong() : 0;
                long version = fileObj.has("version") && !fileObj.get("version").isJsonNull() ? fileObj.get("version").getAsLong() : 0;

                String displayText = String.format("%s (v%d, %s)", path, version, formatBytes(size));
                filesListView.getItems().add(displayText);
            }
        }

        if (selectedIndex >= 0 && selectedIndex < filesListView.getItems().size()) {
            filesListView.getSelectionModel().select(selectedIndex);
        }

        syncStatusLabel.setText("Sync Status: Idle / Up to date");
    }

    private void handlePeerListUpdate(JsonElement payload) {
        if (payload == null || !payload.isJsonObject()) return;

        JsonObject obj = payload.getAsJsonObject();
        if (!obj.has("peers") || obj.get("peers").isJsonNull()) return;

        JsonArray peers = obj.getAsJsonArray("peers");
        int totalPeers = peers.size();
        int connectedPeers = 0;

        for (JsonElement peerEl : peers) {
            if (peerEl != null && peerEl.isJsonObject()) {
                JsonObject peerObj = peerEl.getAsJsonObject();
                if (peerObj.has("connected") && !peerObj.get("connected").isJsonNull() && peerObj.get("connected").getAsBoolean()) {
                    connectedPeers++;
                }
            }
        }

        peersLabel.setText("Peers: " + connectedPeers + " (" + totalPeers + ")");
        if (connectedPeers > 0) {
            onlineStatusLabel.setText("Online");
            onlineStatusLabel.setStyle("-fx-text-fill: -theme-success; -fx-font-weight: bold;");
        } else {
            onlineStatusLabel.setText("Offline");
            onlineStatusLabel.setStyle("-fx-text-fill: -theme-danger; -fx-font-weight: bold;");
        }
    }

    private void handleConflictDetected(JsonElement payload) {
        if (payload == null || !payload.isJsonObject()) return;
        JsonObject obj = payload.getAsJsonObject();
        String path = obj.has("path") && !obj.get("path").isJsonNull() ? obj.get("path").getAsString() : "unknown";

        String localPeerTmp = "you";
        String remotePeerTmp = "unknown";
        if (obj.has("versions") && obj.get("versions").isJsonArray()) {
            JsonArray versions = obj.getAsJsonArray("versions");
            if (versions.size() >= 2) {
                JsonObject v0 = versions.get(0).getAsJsonObject();
                JsonObject v1 = versions.get(1).getAsJsonObject();
                if (v0.has("source_peer") && !v0.get("source_peer").isJsonNull()) localPeerTmp = v0.get("source_peer").getAsString();
                if (v1.has("source_peer") && !v1.get("source_peer").isJsonNull()) remotePeerTmp = v1.get("source_peer").getAsString();
            }
        }
        final String localPeer = localPeerTmp;
        final String remotePeer = remotePeerTmp;

        logToConsole("[CONFLICT] Concurrent edits on: " + path);
        syncStatusLabel.setText("Sync Status: CONFLICT DETECTED");

        Platform.runLater(() -> {
            ConflictDialog dialog = new ConflictDialog(path, "local version", "remote version", localPeer, remotePeer);
            dialog.showAndWait().ifPresent(resolution -> {
                JsonObject resolutionPayload = new JsonObject();
                resolutionPayload.addProperty("repo_id", repoId);
                resolutionPayload.addProperty("path", path);
                resolutionPayload.addProperty("resolution", resolution);
                resolutionPayload.addProperty("peer_id", remotePeer);
                IpcBridge.getInstance().send("conflict_resolution", resolutionPayload);
                logToConsole("[CONFLICT] Resolved: " + resolution + " for " + path);
                syncStatusLabel.setText("Sync Status: Idle / Up to date");
            });
        });
    }

    private void handleSyncFromPeer(JsonElement payload) {
        if (payload == null || !payload.isJsonObject()) return;
        JsonObject obj = payload.getAsJsonObject();
        String path = obj.has("path") && !obj.get("path").isJsonNull() ? obj.get("path").getAsString() : "unknown";
        String peer = obj.has("peer_id") && !obj.get("peer_id").isJsonNull() ? obj.get("peer_id").getAsString() : "unknown";
        boolean isDelete = obj.has("is_delete") && !obj.get("is_delete").isJsonNull() && obj.get("is_delete").getAsBoolean();

        if (isDelete) {
            logToConsole("[DELETE] Applied deletion from peer " + peer + " for file: " + path);
        } else {
            logToConsole("[SYNC] Downloading updated file from peer " + peer + ": " + path);
        }
        syncStatusLabel.setText("Sync Status: Syncing...");
    }

    private void handleFileTransferComplete(JsonElement payload) {
        if (payload == null || !payload.isJsonObject()) return;
        JsonObject obj = payload.getAsJsonObject();
        String path = obj.has("path") && !obj.get("path").isJsonNull() ? obj.get("path").getAsString() : "unknown";
        boolean success = obj.has("success") && !obj.get("success").isJsonNull() && obj.get("success").getAsBoolean();
        String error = obj.has("error") && !obj.get("error").isJsonNull() ? obj.get("error").getAsString() : "";

        if (success) {
            logToConsole("[SYNC COMPLETE] File updated: " + path);
        } else {
            logToConsole("[SYNC FAILED] File: " + path + ". Error: " + error);
        }

        pollStatus();
    }

    private void logToConsole(String message) {
        String timestamp = dateFormat.format(new Date());
        changesTextArea.appendText("[" + timestamp + "] " + message + "\n");
    }

    private String formatBytes(long bytes) {
        if (bytes < 1024) return bytes + " B";
        int exp = (int) (Math.log(bytes) / Math.log(1024));
        char pre = "KMGTPE".charAt(exp - 1);
        return String.format("%.1f %sB", bytes / Math.pow(1024, exp), pre);
    }

    @FXML
    protected void handleAddPeer() {
        javafx.scene.control.Dialog<java.util.Map<String, String>> dialog = new javafx.scene.control.Dialog<>();
        dialog.setTitle("Connect to Peer Manually");
        dialog.setHeaderText("Add peer details when automatic discovery (mDNS) is blocked");

        try {
            dialog.getDialogPane().getStylesheets().addAll(
                P2PApplication.class.getResource("styles.css").toExternalForm()
            );
            dialog.getDialogPane().getStyleClass().add("root");
        } catch (Exception ignored) {}

        javafx.scene.control.ButtonType actionButtonType = new javafx.scene.control.ButtonType("Connect", javafx.scene.control.ButtonBar.ButtonData.OK_DONE);
        dialog.getDialogPane().getButtonTypes().addAll(actionButtonType, javafx.scene.control.ButtonType.CANCEL);

        javafx.scene.layout.GridPane grid = new javafx.scene.layout.GridPane();
        grid.setHgap(10);
        grid.setVgap(10);
        grid.setPadding(new javafx.geometry.Insets(20, 150, 10, 10));

        javafx.scene.control.TextField peerIdField = new javafx.scene.control.TextField();
        peerIdField.setPromptText("e.g. peer-hostname");
        javafx.scene.control.TextField addressField = new javafx.scene.control.TextField();
        addressField.setPromptText("e.g. 192.168.1.15");

        grid.add(new javafx.scene.control.Label("Peer ID:"), 0, 0);
        grid.add(peerIdField, 1, 0);
        grid.add(new javafx.scene.control.Label("IP Address:"), 0, 1);
        grid.add(addressField, 1, 1);

        dialog.getDialogPane().setContent(grid);

        Platform.runLater(peerIdField::requestFocus);

        dialog.setResultConverter(dialogButton -> {
            if (dialogButton == actionButtonType) {
                java.util.Map<String, String> map = new java.util.HashMap<>();
                map.put("peer_id", peerIdField.getText().trim());
                map.put("address", addressField.getText().trim());
                return map;
            }
            return null;
        });

        dialog.showAndWait().ifPresent(result -> {
            String id = result.get("peer_id");
            String address = result.get("address");
            if (id.isEmpty() || address.isEmpty()) {
                Alert alert = new Alert(Alert.AlertType.WARNING, "Peer ID and Address cannot be empty.", ButtonType.OK);
                alert.setTitle("Validation Error");
                alert.showAndWait();
                return;
            }

            int port = 9876;
            String host = address;
            if (address.contains(":")) {
                String[] parts = address.split(":");
                host = parts[0];
                try {
                    port = Integer.parseInt(parts[1]);
                    if (port < 1 || port > 65535) {
                        throw new NumberFormatException();
                    }
                } catch (NumberFormatException e) {
                    Alert alert = new Alert(Alert.AlertType.WARNING, "Invalid port number. Must be 1-65535.", ButtonType.OK);
                    alert.setTitle("Validation Error");
                    alert.showAndWait();
                    return;
                }
            }
            if (host.isEmpty()) {
                Alert alert = new Alert(Alert.AlertType.WARNING, "Host address cannot be empty.", ButtonType.OK);
                alert.setTitle("Validation Error");
                alert.showAndWait();
                return;
            }

            JsonObject payload = new JsonObject();
            payload.addProperty("peer_id", id);
            payload.addProperty("address", host);
            payload.addProperty("port", port);

            IpcBridge.getInstance().send("add_peer", payload);
            logToConsole("[Manual Connect] Sent request to connect to " + id + " at " + host + ":" + port);
        });
    }

    public void shutdown() {
        if (pollTimeline != null) {
            pollTimeline.stop();
        }
        IpcBridge bridge = IpcBridge.getInstance();
        bridge.removeListener("repo_status_response", repoStatusListener);
        bridge.removeListener("peer_list_update", peerListListener);
        bridge.removeListener("conflict_detected", conflictListener);
        bridge.removeListener("sync_from_peer", syncFromPeerListener);
        bridge.removeListener("file_transfer_complete", transferCompleteListener);
    }
}
