# HA Windows Shutdown Client

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go)
![License](https://img.shields.io/badge/License-MIT-blue.svg?style=for-the-badge)
![Platform](https://img.shields.io/badge/Platform-Windows-0078D6?style=for-the-badge&logo=windows)

A lightweight, secure, and modern **Go** application that allows **Home Assistant** to remotely shut down, restart, hibernate, or sleep any Windows computer on your network.

Built with a focus on **security**, **privacy**, and **user experience**, this client runs entirely in user space (no admin rights required) and features a modern, native-looking interface.

## ✨ Key Features

-   🔒 **Military-Grade Security**: API keys are stored encrypted using **AES-256-GCM** in the Windows Registry. No plaintext secrets.
-   🚀 **Zero-Install**: A single static `.exe` binary. No installer, no registry bloat (settings stored in `HKCU`).
-   🔍 **Auto-Discovery**: Automatically advertises itself on the local network via **mDNS (Zeroconf)** for seamless Home Assistant integration.
-   🌐 **Bilingual**: Fully localized in **English** and **Dutch** (switch instantly via system tray).
-   🛡️ **Privacy First**: Runs entirely under the current user account. No background services, no admin privileges needed.

## 📋 Requirements

-   **OS**: Windows 10 or Windows 11 (64-bit)
-   **Home Assistant**: Any version supporting HTTP API calls
-   **Build Environment** (Optional): [Go 1.21+](https://go.dev/dl/)

## 🚀 Quick Start

### 1. Download
Download the latest pre-built binary from the [Releases](https://github.com/yourusername/ha-windows-shutdown-client/releases) page.
*   `ha-shutdown-client.exe` (Release build, no console window)

### 2. Run
Simply double-click `ha-shutdown-client.exe`.
-   The application will start in the background and appear in your **System Tray**.
-   A unique **API Key** is automatically generated and displayed in the console (if run manually) or accessible via the tray menu.

### 3. Configure Home Assistant
1.  Click the **HA Shutdown Client** icon in your system tray.
2.  Select **Connection Info** to view your API Key.
3.  In Home Assistant, go to **Settings → Devices & Services → Add Integration**.
4.  Search for **Windows Shutdown**.
5.  Enter your **Host IP** (or hostname) and the **API Key**.
6.  The integration will automatically discover the device via mDNS.

## 🛠️ Building from Source

If you prefer to build from source or contribute:

```bash
# Clone the repository
git clone https://github.com/yourusername/ha-windows-shutdown-client.git
cd ha-windows-shutdown-client

# Build the release version (optimized, no console window)
go build -ldflags="-s -w -H windowsgui" -o ha-shutdown-client.exe .

# Or build the debug version (with console window)
go build -o ha-shutdown-client.exe .
```
## 📖 Usage & Configuration

### System Tray Menu
Right-click the icon in the system tray to access:
-   **Connection Info**: View Hostname, Port, and API Key.
-   **Settings**: Change the listening port, default delay, and shutdown type.
-   **Language**: Toggle between English and Dutch.
-   **Exit**: Stop the client.

### Command Line Options
Run `ha-shutdown-client.exe --help` for a full list of options:

```text
--port PORT         Listening port (default: 8765)
--delay SECONDS     Default delay before shutdown (default: 30)
--type TYPE         Default shutdown type (shutdown|restart|hibernate|sleep|logoff)
--save-defaults     Save settings to the Windows Registry
--show-password     Display the current API key and exit
--reset-password    Generate a new API key
--no-tray           Run in console mode (no system tray)
```

### Auto-Start with Windows
To start the client automatically when you log in:
1.  Press `Win + R`, type `shell:startup`, and press Enter.
2.  Create a shortcut to `ha-shutdown-client.exe` in the opened folder.

## 🔐 Security Architecture

As a security-focused tool, the client implements several layers of protection:

1.  **Encryption at Rest**: The API key is never stored in plain text. It is encrypted using **AES-256-GCM** with a randomly generated key stored in the registry.
2.  **Timing-Safe Authentication**: The HTTP server uses `crypto/subtle.ConstantTimeCompare` to prevent timing attacks on the API key.
3.  **Input Validation**: Strict validation on all incoming JSON payloads to prevent injection or buffer overflow attempts.
4.  **User-Level Isolation**: All operations run under the current user context (`HKEY_CURRENT_USER`), ensuring no system-wide changes or privilege escalation risks.
5.  **Rate Limiting**: Built-in protection against brute-force attempts on the API endpoints.

## 📡 API Endpoints

The client exposes a simple REST API. All authenticated endpoints require the `X-API-Key` header.

| Method | Endpoint | Description | Auth |
| :--- | :--- | :--- | :--- |
| `GET` | `/status` | Returns online status and hostname | ❌ No |
| `GET` | `/verify` | Validates the API key | ✅ Yes |
| `POST` | `/shutdown` | Triggers a shutdown/restart/etc. | ✅ Yes |

**Example Request:**
```bash
curl -X POST http://localhost:8765/shutdown \
  -H "X-API-Key: your-api-key-here" \
  -H "Content-Type: application/json" \
  -d '{"delay": 30, "type": "shutdown"}'
```

## 🤝 Contributing

Contributions are welcome! Whether it's fixing bugs, adding new shutdown types, or improving the UI, please feel free to submit a Pull Request.

1.  Fork the repository.
2.  Create your feature branch (`git checkout -b feature/AmazingFeature`).
3.  Commit your changes (`git commit -m 'Add some AmazingFeature'`).
4.  Push to the branch (`git push origin feature/AmazingFeature`).
5.  Open a Pull Request.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

-   Built with [Go](https://go.dev/) and [Fyne](https://fyne.io/).
-   Uses [getlantern/systray](https://github.com/getlantern/systray) for the system tray.
-   Uses [grandcat/zeroconf](https://github.com/grandcat/zeroconf) for mDNS discovery.

---

*Made with AI for the Home Assistant community.*