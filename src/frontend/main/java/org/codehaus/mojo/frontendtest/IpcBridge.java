package org.codehaus.mojo.frontendtest;

import com.google.gson.Gson;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import javafx.application.Platform;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.StandardProtocolFamily;
import java.net.UnixDomainSocketAddress;
import java.nio.ByteBuffer;
import java.nio.channels.SocketChannel;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

public class IpcBridge {
    static {
        try {
            java.io.PrintStream logStream = new java.io.PrintStream(new java.io.FileOutputStream("/tmp/p2p_java.log", true));
            System.setOut(logStream);
            System.setErr(logStream);
            System.out.println("\n--- Java Session Started: " + new java.util.Date() + " ---");
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    private static IpcBridge instance;
    private final Gson gson = new Gson();
    private SocketChannel socketChannel;
    private final Map<String, List<MessageListener>> listeners = new ConcurrentHashMap<>();
    private Thread readThread;
    private volatile boolean running = false;
    private Process goProcess;

    public interface MessageListener {
        void onMessage(JsonElement payload);
    }

    private static java.io.File getBaseDir() {
        // 1. Primary: Walk up from java.home (always points to embedded JRE inside app bundle)
        try {
            String javaHome = System.getProperty("java.home");
            if (javaHome != null) {
                java.io.File dir = new java.io.File(javaHome);
                while (dir != null) {
                    if (new java.io.File(dir, "src/backend/go").exists()) {
                        return dir;
                    }
                    dir = dir.getParentFile();
                }
            }
        } catch (Exception e) {
            System.err.println("[Java] Failed walking up from java.home: " + e.getMessage());
        }

        // 2. Fallback: Protection domain URI (only works when not loading from modular jrt:)
        try {
            java.net.URI uri = IpcBridge.class.getProtectionDomain().getCodeSource().getLocation().toURI();
            if (uri != null && uri.getPath() != null) {
                java.io.File pathFile = new java.io.File(uri.getPath());
                java.io.File parent = pathFile.getParentFile();
                while (parent != null) {
                    if (new java.io.File(parent, "src/backend/go").exists()) {
                        return parent;
                    }
                    parent = parent.getParentFile();
                }
            }
        } catch (Exception e) {
            System.err.println("[Java] Failed walking up from protection domain: " + e.getMessage());
        }

        // 3. Last resort
        return new java.io.File(System.getProperty("user.dir"));
    }

    private synchronized void ensureGoCoordinatorRunning() {
        if (goProcess != null && goProcess.isAlive()) {
            return;
        }

        java.io.File baseDir = getBaseDir();
        System.out.println("[Java] Resolved base directory: " + baseDir.getAbsolutePath());

        // 1. Try to build the Go coordinator if in development environment
        java.io.File devGoDir = new java.io.File(baseDir, "src/backend/go");
        if (devGoDir.exists() && devGoDir.isDirectory()) {
            System.out.println("[Java] Development environment detected. Building Go coordinator...");
            try {
                new java.io.File(baseDir, "build").mkdirs();
                ProcessBuilder pbBuild = new ProcessBuilder("go", "build", "-o", "../../../build/go_coordinator", "main.go");
                pbBuild.directory(devGoDir);
                Process buildProc = pbBuild.start();
                int exitCode = buildProc.waitFor();
                if (exitCode == 0) {
                    System.out.println("[Java] Go coordinator built successfully.");
                } else {
                    System.err.println("[Java] Go compilation failed with exit code: " + exitCode);
                }
            } catch (Exception e) {
                System.err.println("[Java] Error compiling Go coordinator: " + e.getMessage());
            }
        }

        // 2. Start Go coordinator
        try {
            java.io.File goExe = new java.io.File(System.getProperty("java.home"), "bin/go_coordinator");
            if (!goExe.exists()) {
                goExe = new java.io.File(baseDir, "build/go_coordinator");
            }
            if (!goExe.exists()) {
                goExe = new java.io.File(baseDir, "src/backend/go/build/go_coordinator");
            }
            if (!goExe.exists()) {
                goExe = new java.io.File(baseDir, "go_coordinator");
            }

            if (goExe.exists()) {
                System.out.println("[Java] Starting Go coordinator: " + goExe.getAbsolutePath());
                ProcessBuilder pbGo = new ProcessBuilder(goExe.getAbsolutePath());
                pbGo.directory(baseDir); // Set working directory to project root!
                
                Map<String, String> env = pbGo.environment();
                String socketPath = "/tmp/p2p_sync.sock";
                if (System.getenv("IPC_SOCKET") != null) {
                    socketPath = System.getenv("IPC_SOCKET");
                }
                env.put("IPC_SOCKET", socketPath);
                env.put("DB_PATH", new java.io.File(baseDir, "p2p_sync.db").getAbsolutePath());
                
                // Redirect output to a log file instead of inheriting in headless/App bundle mode
                java.io.File logFile = new java.io.File("/tmp/p2p_go.log");
                pbGo.redirectOutput(ProcessBuilder.Redirect.to(logFile));
                pbGo.redirectError(ProcessBuilder.Redirect.to(logFile));
                
                goProcess = pbGo.start();
                System.out.println("[Java] Go coordinator started in background.");
            } else {
                System.err.println("[Java] Go coordinator binary not found! Cannot start.");
            }
        } catch (Exception e) {
            System.err.println("[Java] Error starting Go coordinator process: " + e.getMessage());
        }
    }

    private IpcBridge() {}

    public static synchronized IpcBridge getInstance() {
        if (instance == null) {
            instance = new IpcBridge();
        }
        return instance;
    }

    public synchronized void connect() {
        if (running) return;

        ensureGoCoordinatorRunning();

        running = true;
        readThread = new Thread(this::connectionLoop, "IPC-Reader-Thread");
        readThread.setDaemon(true);
        readThread.start();
    }

    private void connectionLoop() {
        while (running) {
            try {
                if (socketChannel == null || !socketChannel.isOpen() || !socketChannel.isConnected()) {
                    socketChannel = tryConnect();
                }

                if (socketChannel != null && socketChannel.isConnected()) {
                    readMessages(socketChannel);
                }
            } catch (Exception e) {
                System.err.println("IPC Connection error: " + e.getMessage());
                try {
                    if (socketChannel != null) {
                        socketChannel.close();
                    }
                } catch (IOException ignored) {}
                socketChannel = null;
            }

            if (running) {
                try {
                    // Try to reconnect every 2 seconds if disconnected
                    Thread.sleep(2000);
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    break;
                }
            }
        }
    }

    private SocketChannel tryConnect() {
        // 1. Try Unix Domain Socket
        try {
            String socketPath = "/tmp/p2p_sync.sock";
            if (System.getenv("IPC_SOCKET") != null) {
                socketPath = System.getenv("IPC_SOCKET");
            }
            System.out.println("Connecting to UNIX Domain Socket at " + socketPath + "...");
            UnixDomainSocketAddress address = UnixDomainSocketAddress.of(socketPath);
            SocketChannel channel = SocketChannel.open(StandardProtocolFamily.UNIX);
            channel.connect(address);
            System.out.println("Connected to UNIX domain socket.");
            return channel;
        } catch (Exception e) {
            System.err.println("UNIX Domain Socket connection failed: " + e.getMessage());
        }

        // 2. Try TCP fallback
        try {
            System.out.println("Trying TCP fallback on 127.0.0.1:9999...");
            SocketChannel channel = SocketChannel.open();
            channel.connect(new InetSocketAddress("127.0.0.1", 9999));
            System.out.println("Connected to TCP socket.");
            return channel;
        } catch (Exception e) {
            System.err.println("TCP fallback connection failed: " + e.getMessage());
        }

        return null;
    }

    private void readMessages(SocketChannel channel) throws IOException {
        ByteBuffer lenBuf = ByteBuffer.allocate(4);

        while (running && channel.isConnected()) {
            lenBuf.clear();
            while (lenBuf.hasRemaining()) {
                int read = channel.read(lenBuf);
                if (read == -1) {
                    throw new IOException("EOF reached");
                }
            }
            lenBuf.flip();
            int length = lenBuf.getInt();

            if (length <= 0 || length > 10 * 1024 * 1024) { // 10MB sanity check
                throw new IOException("Invalid message length: " + length);
            }

            ByteBuffer msgBuf = ByteBuffer.allocate(length);
            while (msgBuf.hasRemaining()) {
                int read = channel.read(msgBuf);
                if (read == -1) {
                    throw new IOException("EOF reached reading payload");
                }
            }
            msgBuf.flip();

            byte[] bytes = new byte[length];
            msgBuf.get(bytes);
            String jsonStr = new String(bytes, java.nio.charset.StandardCharsets.UTF_8);

            try {
                JsonObject msg = gson.fromJson(jsonStr, JsonObject.class);
                String type = msg.get("type").getAsString();
                JsonElement payload = msg.get("payload");

                List<MessageListener> list = listeners.get(type);
                if (list != null) {
                    for (MessageListener listener : list) {
                        Platform.runLater(() -> listener.onMessage(payload));
                    }
                }
            } catch (Exception e) {
                System.err.println("Failed to parse/dispatch message: " + e.getMessage());
            }
        }
    }

    public synchronized void send(String type, Object payload) {
        if (socketChannel == null || !socketChannel.isConnected()) {
            System.err.println("Cannot send message: not connected to IPC server.");
            return;
        }

        JsonObject msg = new JsonObject();
        msg.addProperty("version", "1.0");
        msg.addProperty("type", type);
        msg.addProperty("id", "msg_java_" + UUID.randomUUID().toString().replace("-", "").substring(0, 16));
        msg.addProperty("timestamp", System.currentTimeMillis());
        msg.addProperty("source", "java");
        msg.add("payload", gson.toJsonTree(payload));

        String jsonStr = gson.toJson(msg);
        byte[] bytes = jsonStr.getBytes(java.nio.charset.StandardCharsets.UTF_8);

        ByteBuffer buf = ByteBuffer.allocate(4 + bytes.length);
        buf.putInt(bytes.length);
        buf.put(bytes);
        buf.flip();

        try {
            while (buf.hasRemaining()) {
                socketChannel.write(buf);
            }
        } catch (IOException e) {
            System.err.println("Failed to send message: " + e.getMessage());
        }
    }

    public void registerListener(String type, MessageListener listener) {
        listeners.computeIfAbsent(type, k -> new ArrayList<>()).add(listener);
    }

    public void disconnect() {
        running = false;
        if (readThread != null) {
            readThread.interrupt();
        }
        try {
            if (socketChannel != null) {
                socketChannel.close();
            }
        } catch (IOException ignored) {}
        socketChannel = null;

        if (goProcess != null && goProcess.isAlive()) {
            System.out.println("[Java] Terminating Go coordinator process...");
            goProcess.destroy();
            try {
                goProcess.waitFor();
            } catch (InterruptedException ignored) {}
            goProcess = null;
        }
    }
}
