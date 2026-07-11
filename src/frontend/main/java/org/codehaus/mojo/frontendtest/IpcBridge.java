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
            String tmpDir = System.getProperty("java.io.tmpdir");
            java.io.File logFile = new java.io.File(tmpDir, "p2p_java.log");
            java.io.PrintStream logStream = new java.io.PrintStream(new java.io.FileOutputStream(logFile, true));
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
    private int reconnectFailures;
    private String resolvedSocketPath;
    private String resolvedDbPath;

    public interface MessageListener {
        void onMessage(JsonElement payload);
    }

    /** Number of consecutive connection failures before restarting the Go coordinator. */
    private static final int MAX_RECONNECT_FAILURES = 3;

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

        // 1. Skip Go build if SKIP_GO_BUILD env var is set (production mode)
        boolean skipGoBuild = "1".equals(System.getenv("SKIP_GO_BUILD")) || "true".equals(System.getenv("SKIP_GO_BUILD"));
        if (!skipGoBuild) {
            java.io.File devGoDir = new java.io.File(baseDir, "src/backend/go");
            if (devGoDir.exists() && devGoDir.isDirectory()) {
                System.out.println("[Java] Development environment detected. Building Go coordinator...");
                try {
                    new java.io.File(baseDir, "build").mkdirs();
                    String outputName = System.getProperty("os.name").toLowerCase().contains("win") ? "go_coordinator.exe" : "go_coordinator";
                    ProcessBuilder pbBuild = new ProcessBuilder("go", "build", "-o", "../../../build/" + outputName, "main.go");
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
        } else {
            System.out.println("[Java] SKIP_GO_BUILD set — assuming pre-built Go coordinator binary exists.");
        }

        // 2. Start Go coordinator
        try {
            String exeSuffix = System.getProperty("os.name").toLowerCase().contains("win") ? ".exe" : "";
            java.io.File goExe = new java.io.File(System.getProperty("java.home"), "bin/go_coordinator" + exeSuffix);
            if (!goExe.exists()) {
                goExe = new java.io.File(baseDir, "build/go_coordinator" + exeSuffix);
            }
            if (!goExe.exists()) {
                goExe = new java.io.File(baseDir, "src/backend/go/build/go_coordinator" + exeSuffix);
            }
            if (!goExe.exists()) {
                goExe = new java.io.File(baseDir, "go_coordinator" + exeSuffix);
            }

            if (goExe.exists()) {
                System.out.println("[Java] Starting Go coordinator: " + goExe.getAbsolutePath());
                ProcessBuilder pbGo = new ProcessBuilder(goExe.getAbsolutePath());
                pbGo.directory(baseDir); // Set working directory to project root!
                
                Map<String, String> env = pbGo.environment();
                env.put("IPC_SOCKET", resolvedSocketPath);
                env.put("DB_PATH", resolvedDbPath);
                
                // Redirect output to a log file instead of inheriting in headless/App bundle mode
                java.io.File logFile = new java.io.File(System.getProperty("java.io.tmpdir"), "p2p_go.log");
                pbGo.redirectOutput(ProcessBuilder.Redirect.to(logFile));
                pbGo.redirectError(ProcessBuilder.Redirect.to(logFile));
                
                goProcess = pbGo.start();
                System.out.println("[Java] Go coordinator started in background (PID: " + goProcess.pid() + ").");
                
                // Quick health check: wait briefly and verify process is still alive
                try {
                    Thread.sleep(500);
                    if (!goProcess.isAlive()) {
                        int exitVal = goProcess.exitValue();
                        System.err.println("[Java] Go coordinator exited immediately with code " + exitVal + ". Check " + logFile.getAbsolutePath() + " for details.");
                    }
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                }
            } else {
                System.err.println("[Java] Go coordinator binary not found! Cannot start.");
            }
        } catch (Exception e) {
            System.err.println("[Java] Error starting Go coordinator process: " + e.getMessage());
        }
    }

    private IpcBridge() {
        // Resolve socket path
        String tmpDir = System.getProperty("java.io.tmpdir");
        String socketPath = new java.io.File(tmpDir, "p2p_sync.sock").getAbsolutePath();
        if (System.getenv("IPC_SOCKET") != null) {
            socketPath = System.getenv("IPC_SOCKET");
        } else {
            // Include username to avoid multi-user permission conflicts on the same machine
            socketPath = new java.io.File(tmpDir, "p2p_sync_" + System.getProperty("user.name") + ".sock").getAbsolutePath();
        }
        this.resolvedSocketPath = socketPath;

        // Resolve database path
        String dbPath;
        if (System.getenv("DB_PATH") != null) {
            dbPath = System.getenv("DB_PATH");
        } else {
            String userHome = System.getProperty("user.home");
            java.io.File dbDir;
            String os = System.getProperty("os.name").toLowerCase();
            if (os.contains("mac")) {
                dbDir = new java.io.File(userHome, "Library/Application Support/P2PVersionControl");
            } else if (os.contains("win")) {
                dbDir = new java.io.File(System.getenv("APPDATA"), "P2PVersionControl");
            } else {
                dbDir = new java.io.File(userHome, ".config/P2PVersionControl");
            }
            dbDir.mkdirs();
            dbPath = new java.io.File(dbDir, "p2p_sync.db").getAbsolutePath();
        }
        this.resolvedDbPath = dbPath;
    }

    public static synchronized IpcBridge getInstance() {
        if (instance == null) {
            instance = new IpcBridge();
            Runtime.getRuntime().addShutdownHook(new Thread(() -> {
                System.out.println("[Java] Shutdown hook triggered. Stopping sub-processes...");
                if (instance != null) {
                    instance.disconnect();
                }
            }));
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
                    reconnectFailures = 0;
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

            if (!running) break;

            reconnectFailures++;
            if (reconnectFailures >= MAX_RECONNECT_FAILURES) {
                System.err.println("[Java] " + MAX_RECONNECT_FAILURES + " consecutive connection failures. Restarting Go coordinator...");
                reconnectFailures = 0;
                synchronized (this) {
                    if (goProcess != null && goProcess.isAlive()) {
                        goProcess.destroy();
                        try {
                            boolean exited = goProcess.waitFor(3, java.util.concurrent.TimeUnit.SECONDS);
                            if (!exited) {
                                goProcess.destroyForcibly();
                                goProcess.waitFor(2, java.util.concurrent.TimeUnit.SECONDS);
                            }
                        } catch (InterruptedException ignored) {
                            goProcess.destroyForcibly();
                        }
                        goProcess = null;
                    }
                    ensureGoCoordinatorRunning();
                }
            }

            try {
                // Throttle reconnect attempts
                Thread.sleep(2000);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            }
        }
    }

    private static int deriveFallbackPort(String socketPath) {
        if (socketPath == null || socketPath.isEmpty()) {
            return 9999;
        }
        byte[] bytes = socketPath.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        long h = 2166136261L;
        for (byte b : bytes) {
            h = (h ^ (b & 0xFF)) * 16777619L;
            h = h & 0xFFFFFFFFL;
        }
        return 10000 + (int) (h % 20000);
    }

    private SocketChannel tryConnect() {
        // 1. Try Unix Domain Socket
        try {
            System.out.println("Connecting to UNIX Domain Socket at " + resolvedSocketPath + "...");
            UnixDomainSocketAddress address = UnixDomainSocketAddress.of(resolvedSocketPath);
            SocketChannel channel = SocketChannel.open(StandardProtocolFamily.UNIX);
            channel.connect(address);
            System.out.println("Connected to UNIX domain socket.");
            return channel;
        } catch (Exception e) {
            System.err.println("UNIX Domain Socket connection failed: " + e.getMessage());
        }

        // 2. Try TCP fallback
        try {
            int port = deriveFallbackPort(resolvedSocketPath);
            System.out.println("Trying TCP fallback on 127.0.0.1:" + port + "...");
            SocketChannel channel = SocketChannel.open();
            channel.connect(new InetSocketAddress("127.0.0.1", port));
            System.out.println("Connected to TCP socket on port " + port + ".");
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
                        Platform.runLater(() -> {
                            try {
                                listener.onMessage(payload);
                            } catch (Exception e) {
                                System.err.println("[Java] Listener exception for type '" + type + "': " + e.getMessage());
                            }
                        });
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

    public void removeListener(String type, MessageListener listener) {
        List<MessageListener> list = listeners.get(type);
        if (list != null) {
            list.remove(listener);
        }
    }

    public boolean isConnected() {
        SocketChannel channel = socketChannel;
        return channel != null && channel.isConnected();
    }

    public synchronized void disconnect() {
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
            System.out.println("[Java] Terminating Go coordinator process (PID: " + goProcess.pid() + ")...");
            goProcess.destroy(); // SIGTERM
            try {
                boolean exited = goProcess.waitFor(5, java.util.concurrent.TimeUnit.SECONDS);
                if (!exited) {
                    System.out.println("[Java] Go coordinator did not exit gracefully. Force killing...");
                    goProcess.destroyForcibly(); // SIGKILL
                    goProcess.waitFor(2, java.util.concurrent.TimeUnit.SECONDS);
                }
            } catch (InterruptedException ignored) {
                goProcess.destroyForcibly();
            }
            goProcess = null;
        }

        // Clean up stale IPC socket and PID file for next startup
        try {
            java.nio.file.Files.deleteIfExists(java.nio.file.Paths.get(resolvedSocketPath));
            String derivedPidPath = resolvedSocketPath.endsWith(".sock") ? 
                resolvedSocketPath.substring(0, resolvedSocketPath.length() - 5) + ".pid" : 
                resolvedSocketPath + ".pid";
            java.nio.file.Files.deleteIfExists(java.nio.file.Paths.get(derivedPidPath));
        } catch (IOException e) {
            System.err.println("[Java] Warning: Could not clean up temp files: " + e.getMessage());
        }
    }
}
