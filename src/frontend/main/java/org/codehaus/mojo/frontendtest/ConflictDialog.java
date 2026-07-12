package org.codehaus.mojo.frontendtest;

import javafx.scene.control.*;
import javafx.scene.layout.*;
import javafx.geometry.Insets;

public class ConflictDialog extends Dialog<String> {
    public ConflictDialog(String filePath, String localVersion, String remoteVersion,
                         String localPeer, String remotePeer) {
        setTitle("Conflict Detected");
        setHeaderText("Concurrent edit detected on: " + filePath);

        try {
            getDialogPane().getStylesheets().addAll(
                HelloApplication.class.getResource("styles.css").toExternalForm(),
                HelloApplication.class.getResource("dark.css").toExternalForm()
            );
            getDialogPane().getStyleClass().add("root");
        } catch (Exception ignored) {}

        VBox content = new VBox(10);
        content.setPadding(new Insets(20));

        Label info = new Label("This file was modified by " + localPeer + " (you) and " + remotePeer + " simultaneously.");

        if (!localVersion.isEmpty() && !remoteVersion.isEmpty()) {
            Label versions = new Label("Local hash: " + localVersion + "\nRemote hash: " + remoteVersion);
            versions.setStyle("-fx-text-fill: -theme-text-muted; -fx-font-family: 'JetBrains Mono', 'Courier New', monospace;");
            content.getChildren().add(versions);
        }

        ToggleGroup group = new ToggleGroup();
        RadioButton keepLocal = new RadioButton("Keep Local - Your version wins");
        RadioButton acceptRemote = new RadioButton("Accept Remote - Their version wins");
        RadioButton manualMerge = new RadioButton("Mark for Manual Merge");
        keepLocal.setToggleGroup(group);
        acceptRemote.setToggleGroup(group);
        manualMerge.setToggleGroup(group);
        keepLocal.setSelected(true);

        content.getChildren().addAll(info, keepLocal, acceptRemote, manualMerge);
        getDialogPane().setContent(content);

        ButtonType resolveBtn = new ButtonType("Resolve", ButtonBar.ButtonData.OK_DONE);
        getDialogPane().getButtonTypes().addAll(resolveBtn, ButtonType.CANCEL);

        setResultConverter(button -> {
            if (button == resolveBtn) {
                if (keepLocal.isSelected()) return "local";
                if (acceptRemote.isSelected()) return "remote";
                return "merge";
            }
            return null;
        });
    }
}
