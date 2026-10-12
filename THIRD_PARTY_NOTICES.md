# Third-party notices

The executable embeds pinned official amneziawg-go packages (MIT) with two documented patches. Preserve `licenses/AmneziaWG-MIT.txt` when redistributing. This code includes WireGuard authors' notices from upstream.

Go runtime dependencies include golang.org/x/crypto, golang.org/x/net and golang.org/x/sys (BSD-style licenses in `licenses`). Wintun and the separate official WireGuard Go module are used by host builds/tests; their license files are included as applicable. The production Android ARM64 executable does not contain a Windows driver.

Playwright is a development-only dependency governed by its package license; it is not embedded in the distributed JS. The JS runtime uses browser APIs without external libraries.

The official AmneziaWG tools (GPLv2), kernel module, Android APK/JNI binaries, KANO APK, previous WireGuard plugin code and previous draft management code are not redistributed in this deliverable. KANO is the user's host application and remains a separate product.
