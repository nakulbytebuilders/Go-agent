====================================================================
  SILENT BACKGROUND MONITORING SERVICE - SETUP GUIDE FOR ANOTHER PC
====================================================================

1-CLICK SILENT INSTALLATION (NO CONSOLE WINDOW, AUTO-START ON BOOT):

1. Copy this 'dist' folder to the target PC.

2. Double-click 'install.bat'.

THAT'S IT! 
- The agent compiles/runs 100% silently in the background (no black command prompt window).
- It automatically registers to start whenever the PC boots up.
- The web dashboard is immediately active at:
  http://localhost:8080

REMOTE NETWORK DASHBOARD ACCESS:
To access the dashboard from another PC/laptop/phone on the same Wi-Fi:
1. Run 'ipconfig' in Command Prompt on the target PC to get its IP address (e.g. 192.168.1.15).
2. Open any browser on your device and navigate to: http://192.168.1.15:8080

TO UNINSTALL:
- Double-click 'uninstall.bat' to stop the process and remove Windows startup entry.
====================================================================
