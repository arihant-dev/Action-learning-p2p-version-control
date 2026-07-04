package org.codehaus.mojo.frontendtest;

import javafx.collections.FXCollections;
import javafx.collections.ObservableList;
import javafx.fxml.FXML;
import javafx.fxml.FXMLLoader;
import javafx.scene.Scene;
import javafx.scene.control.Label;
import javafx.scene.control.ListView;
import javafx.scene.layout.GridPane;
import javafx.stage.Stage;

public class HelloController {
    @FXML
    private ListView<String> repoListView;

    @FXML
    private Label welcomeText;

    @FXML
    public void initialize(){


    }
    @FXML
    protected void onHelloButtonClick() {





        try {
            FXMLLoader fxmlLoader = new FXMLLoader(HelloApplication.class.getResource("repositoryList.fxml"));
            Scene scene = new Scene(fxmlLoader.load(), 320, 240);

            Stage stage = new Stage();
            stage.setTitle("Hello!");
            stage.setScene(scene);
            stage.show();
        }
        catch (Exception e) {
            e.printStackTrace();
        }

    }
}
