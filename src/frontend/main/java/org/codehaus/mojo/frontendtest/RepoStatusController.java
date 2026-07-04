package org.codehaus.mojo.frontendtest;

import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import javafx.animation.KeyFrame;
import javafx.animation.Timeline;
import javafx.application.Platform;
import javafx.fxml.FXML;
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

        // Immediate poll
        pollStatus();
    }

    public void initialize() {
        // Register listeners for responses
        IpcBridge bridge = IpcBridge.getInstance();
        bridge.registerListener("repo_status_response", repoStatusListener);
        bridge.registerListener("peer_list_update", peerListListener);
        bridge.registerListener("conflict_detected", conflictListener);
        bridge.registerListener("sync_from_peer", syncFromPeerListener);
        bridge.registerListener("file_transfer_complete", transferCompleteListener);

        // Set up periodic polling for file statuses & peer lists
        pollTimeline = new Timeline(new KeyFrame(Duration.seconds(2.0), event -> pollStatus()));
        pollTimeline.setCycleCount(Timeline.INDEFINITE);
        pollTimeline.play();

        // Request initial peer list
        bridge.send("peer_list_request", new Object());
        
        logToConsole("Status window initialized. Connecting to sync daemon...");

        // Unregister listeners when the window closes
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
        if (!obj.has("repo_id") || !obj.get("repo_id").getAsString().equals(repoId)) {
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
            if (fileEl.isJsonObject()) {
                JsonObject fileObj = fileEl.getAsJsonObject();
                String path = fileObj.get("path").getAsString();
                long size = fileObj.get("size").getAsLong();
                long version = fileObj.get("version").getAsLong();
                
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
        if (!obj.has("peers")) return;

        JsonArray peers = obj.getAsJsonArray("peers");
        int totalPeers = peers.size();
        int connectedPeers = 0;

        for (JsonElement peerEl : peers) {
            if (peerEl.isJsonObject() && peerEl.getAsJsonObject().get("connected").getAsBoolean()) {
                connectedPeers++;
            }
        }

        peersLabel.setText("Peers: " + connectedPeers + " (" + totalPeers + ")");
        if (connectedPeers > 0) {
            onlineStatusLabel.setText("Online");
            onlineStatusLabel.setStyle("-fx-text-fill: #10b981; -fx-font-weight: bold;"); // emerald Green
        } else {
            onlineStatusLabel.setText("Offline");
            onlineStatusLabel.setStyle("-fx-text-fill: #ef4444; -fx-font-weight: bold;"); // Red
        }
    }

    private void handleConflictDetected(JsonElement payload) {
        if (payload == null || !payload.isJsonObject()) return;
        JsonObject obj = payload.getAsJsonObject();
        String path = obj.has("path") ? obj.get("path").getAsString() : "unknown";

        logToConsole("[CONFLICT] Concurrent edits on: " + path);
        logToConsole("Please resolve manually in the directory.");
        syncStatusLabel.setText("Sync Status: CONFLICT DETECTED");
    }

    private void handleSyncFromPeer(JsonElement payload) {
        if (payload == null || !payload.isJsonObject()) return;
        JsonObject obj = payload.getAsJsonObject();
        String path = obj.has("path") ? obj.get("path").getAsString() : "unknown";
        String peer = obj.has("peer_id") ? obj.get("peer_id").getAsString() : "unknown";
        boolean isDelete = obj.has("is_delete") && obj.get("is_delete").getAsBoolean();

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
        String path = obj.has("path") ? obj.get("path").getAsString() : "unknown";
        boolean success = obj.has("success") && obj.get("success").getAsBoolean();
        String error = obj.has("error") ? obj.get("error").getAsString() : "";

        if (success) {
            logToConsole("[SYNC COMPLETE] File updated: " + path);
        } else {
            logToConsole("[SYNC FAILED] File: " + path + ". Error: " + error);
        }
        
        // Immediate status refresh
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
