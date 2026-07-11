package org.codehaus.mojo.frontendtest;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import javafx.geometry.Insets;
import javafx.scene.Scene;
import javafx.scene.control.*;
import javafx.scene.layout.GridPane;
import javafx.scene.layout.VBox;
import javafx.stage.Stage;

import java.io.File;
import java.io.FileReader;
import java.io.FileWriter;
import java.nio.file.Paths;

public class SettingsDialog {

    private static final Gson GSON = new GsonBuilder().setPrettyPrinting().create();
    private static File settingsFile;
    private static JsonObject settings;

    private static File getSettingsFile() {
        if (settingsFile == null) {
            String os = System.getProperty("os.name").toLowerCase();
            String configDir;
            if (os.contains("mac")) {
                configDir = System.getProperty("user.home") + "/Library/Application Support/P2PVersionControl";
            } else if (os.contains("win")) {
                configDir = System.getenv("APPDATA") + "/P2PVersionControl";
            } else {
                configDir = System.getProperty("user.home") + "/.config/P2PVersionControl";
            }
            new File(configDir).mkdirs();
            settingsFile = new File(configDir, "settings.json");
        }
        return settingsFile;
    }

    public static synchronized JsonObject loadSettings() {
        if (settings != null) return settings;
        settings = new JsonObject();
        settings.addProperty("theme", "dark");
        settings.addProperty("auto_start", false);
        settings.addProperty("log_level", "INFO");
        settings.addProperty("ipc_socket", "");
        settings.addProperty("p2p_port", 9876);
        File file = getSettingsFile();
        if (file.exists()) {
            try (FileReader reader = new FileReader(file)) {
                settings = JsonParser.parseReader(reader).getAsJsonObject();
            } catch (Exception e) {
                System.err.println("Failed to load settings: " + e.getMessage());
            }
        }
        return settings;
    }

    public static synchronized void saveSettings(JsonObject newSettings) {
        settings = newSettings;
        File file = getSettingsFile();
        try (FileWriter writer = new FileWriter(file)) {
            GSON.toJson(settings, writer);
        } catch (Exception e) {
            System.err.println("Failed to save settings: " + e.getMessage());
        }
    }

    public static synchronized String getSetting(String key, String defaultValue) {
        if (settings == null) loadSettings();
        return settings.has(key) ? settings.get(key).getAsString() : defaultValue;
    }

    public static synchronized boolean getSetting(String key, boolean defaultValue) {
        if (settings == null) loadSettings();
        return settings.has(key) ? settings.get(key).getAsBoolean() : defaultValue;
    }

    public static synchronized int getSetting(String key, int defaultValue) {
        if (settings == null) loadSettings();
        return settings.has(key) ? settings.get(key).getAsInt() : defaultValue;
    }

    public static void show(Stage owner) {
        JsonObject currentSettings = loadSettings();

        Stage stage = new Stage();
        stage.initOwner(owner);
        stage.setTitle("Settings");

        VBox root = new VBox(15);
        root.setPadding(new Insets(20));

        TabPane tabPane = new TabPane();

        Tab generalTab = new Tab("General");
        VBox generalBox = new VBox(10);
        generalBox.setPadding(new Insets(10));

        CheckBox autoStartCheck = new CheckBox("Auto-start on login");
        autoStartCheck.setSelected(getSetting("auto_start", false));

        ComboBox<String> logLevelCombo = new ComboBox<>();
        logLevelCombo.getItems().addAll("TRACE", "DEBUG", "INFO", "WARN", "ERROR");
        logLevelCombo.setValue(getSetting("log_level", "INFO"));
        GridPane generalGrid = new GridPane();
        generalGrid.setHgap(10);
        generalGrid.setVgap(10);
        generalGrid.add(new Label("Log Level:"), 0, 0);
        generalGrid.add(logLevelCombo, 1, 0);
        generalBox.getChildren().addAll(autoStartCheck, generalGrid);
        generalTab.setContent(generalBox);
        generalTab.setClosable(false);

        Tab networkTab = new Tab("Network");
        VBox networkBox = new VBox(10);
        networkBox.setPadding(new Insets(10));

        String defaultSocket = new java.io.File(System.getProperty("java.io.tmpdir"), "p2p_sync.sock").getAbsolutePath();
        TextField ipcSocketField = new TextField(getSetting("ipc_socket", defaultSocket));
        ipcSocketField.setPromptText("IPC Socket Path");

        TextField p2pPortField = new TextField(String.valueOf(getSetting("p2p_port", 9876)));
        p2pPortField.setPromptText("P2P Port");

        GridPane networkGrid = new GridPane();
        networkGrid.setHgap(10);
        networkGrid.setVgap(10);
        networkGrid.add(new Label("IPC Socket:"), 0, 0);
        networkGrid.add(ipcSocketField, 1, 0);
        networkGrid.add(new Label("P2P Port:"), 0, 1);
        networkGrid.add(p2pPortField, 1, 1);
        networkBox.getChildren().add(networkGrid);
        networkTab.setContent(networkBox);
        networkTab.setClosable(false);

        Tab appearanceTab = new Tab("Appearance");
        VBox appearanceBox = new VBox(10);
        appearanceBox.setPadding(new Insets(10));

        ComboBox<String> themeCombo = new ComboBox<>();
        themeCombo.getItems().addAll("dark", "light");
        themeCombo.setValue(getSetting("theme", "dark"));
        GridPane appearanceGrid = new GridPane();
        appearanceGrid.setHgap(10);
        appearanceGrid.setVgap(10);
        appearanceGrid.add(new Label("Theme:"), 0, 0);
        appearanceGrid.add(themeCombo, 1, 0);
        appearanceBox.getChildren().add(appearanceGrid);
        appearanceTab.setContent(appearanceBox);
        appearanceTab.setClosable(false);

        tabPane.getTabs().addAll(generalTab, networkTab, appearanceTab);

        ButtonBar buttonBar = new ButtonBar();
        Button saveBtn = new Button("Save");
        Button cancelBtn = new Button("Cancel");
        buttonBar.getButtons().addAll(saveBtn, cancelBtn);

        root.getChildren().addAll(tabPane, buttonBar);

        Scene scene = new Scene(root, 420, 320);
        try {
            scene.getStylesheets().add(HelloApplication.class.getResource("styles.css").toExternalForm());
            String activeTheme = getSetting("theme", "dark");
            scene.getStylesheets().add(HelloApplication.class.getResource(activeTheme + ".css").toExternalForm());
        } catch (Exception e) {
            System.err.println("Failed to load stylesheet: " + e.getMessage());
        }

        stage.setScene(scene);

        saveBtn.setOnAction(e -> {
            JsonObject newSettings = new JsonObject();
            newSettings.addProperty("theme", themeCombo.getValue());
            newSettings.addProperty("auto_start", autoStartCheck.isSelected());
            newSettings.addProperty("log_level", logLevelCombo.getValue());
            newSettings.addProperty("ipc_socket", ipcSocketField.getText().trim());
            try {
                newSettings.addProperty("p2p_port", Integer.parseInt(p2pPortField.getText().trim()));
            } catch (NumberFormatException ex) {
                newSettings.addProperty("p2p_port", 9876);
            }
            saveSettings(newSettings);
            stage.close();
        });

        cancelBtn.setOnAction(e -> stage.close());

        stage.show();
    }

    public static void applyTheme(Scene scene) {
        String theme = getSetting("theme", "dark");
        scene.getStylesheets().removeIf(s -> s.contains("dark.css") || s.contains("light.css"));
        scene.getStylesheets().add(HelloApplication.class.getResource(theme + ".css").toExternalForm());
    }
}
