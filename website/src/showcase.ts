/**
 * Apps built with MyGo, shown at /showcase. To add yours, open a pull request
 * that adds an entry to the end of this list.
 *
 * - name: the app's name.
 * - description: what it does, in a sentence or two.
 * - url: where to get it or read about it, such as its repository.
 * - author: optional, your name or GitHub login.
 * - mode: optional, "native" when its windows draw native UI (package ui),
 *   "web" when they show a web frontend in the system webview.
 * - image: optional, a screenshot. Put it in website/public/showcase/ and
 *   give its path, such as "/showcase/my-app.png" (about 1600×1000, under
 *   300 KB).
 */
export interface ShowcaseApp {
  name: string
  description: string
  url: string
  author?: string
  mode?: "native" | "web"
  image?: string
}

export const showcase: ShowcaseApp[] = [
  {
    name: "MixMusic",
    description:
      "An open-source music player for macOS, Windows and Linux. One search looks through YouTube Music and Spotify, and one queue and library mix tracks from both.",
    url: "https://github.com/fzn0x/mixmusic",
    author: "fzn0x",
    mode: "native",
    image: "/showcase/mixmusic.jpg",
  },
  {
    name: "Pulse",
    description:
      "An activity monitor for Mac, Windows and Linux. Every process with its app's icon, each CPU core, memory pressure, the GPU, disks, network connections, energy and temperatures, with live readings in the menu bar.",
    url: "https://pulse.egoist.dev",
    author: "egoist",
    mode: "native",
    image: "/showcase/pulse.jpg",
  },
  {
    name: "Kero",
    description:
      "A terminal for coding agents. Sessions keep running when you quit, a sidebar shows which agents are working, which need you and which are done, and a CLI lets agents drive it, on your machine or a remote host.",
    url: "https://kero.sh",
    author: "egoist",
    mode: "native",
  },
  {
    name: "Lorca",
    description:
      "AI teammates on your computer. Chat with one bot or put several in a group chat; they read and edit files, run commands and hand work to each other, and chats sync end-to-end encrypted to your other devices.",
    url: "https://lorca.app",
    author: "egoist",
    mode: "native",
  },
  {
    name: "MyGo 云音乐",
    description:
      "A third-party NetEase Cloud Music client. Daily picks, playlists, search, personal FM, word-by-word lyrics, lossless and Hi-Res playback, podcasts with on-device transcripts, and a mini player in the notch.",
    url: "https://cloudmusic.egoist.dev",
    author: "egoist",
    mode: "native",
  },
  {
    name: "GoRex",
    description:
      "A replica of Superlogical's Rex terminal. Tabs and split panes as cards, sessions that keep running after you quit, and each pane's program and activity in its header.",
    url: "https://github.com/egoist/gorex",
    author: "egoist",
    mode: "native",
    image: "/showcase/gorex.jpg",
  },
  {
    name: "Godiff",
    description:
      "A minimal diff viewer for reviewing Git changes and committing them. Split and unified diffs, review comments to copy for an agent, history, and GitHub pull request reviews.",
    url: "https://github.com/egoist/godiff",
    author: "egoist",
    mode: "native",
  },
  {
    name: "2048",
    description:
      "The classic 2048 puzzle, played with the arrow keys, a drag or a touchpad swipe. Its WebAssembly build runs in the browser too.",
    url: "https://github.com/egoist/2048",
    author: "egoist",
    mode: "native",
    image: "/showcase/2048.jpg",
  },
  {
    name: "Uma",
    description: "A menu bar app that shows the websites of an Umami account and their visitors.",
    url: "https://github.com/egoist/uma",
    author: "egoist",
    mode: "native",
  },
  {
    name: "留白 Liubai",
    description:
      "An open-source Markdown editor and reader for macOS, with a Windows beta. The page formats as you type, the finished document reads beside it or on its own, files stay plain .md, and eleven themes follow the hours of the day or the seasons.",
    url: "https://liubai.echovic.com",
    author: "echoVic",
    mode: "web",
    image: "/showcase/liubai.jpg",
  },
  {
    name: "Clipnest",
    description:
      "An open-source clipboard history for Mac. It keeps the text, links, code and images you copy and the snippets you reuse; press ⌘⇧V to pick one and paste it back where you were.",
    url: "https://clipnest.echovic.com",
    author: "echoVic",
    mode: "web",
    image: "/showcase/clipnest.jpg",
  },
  {
    name: "Cleanzo",
    description:
      "An open-source disk cleaner for macOS. It clears junk, shows what fills the disk, finds large files and similar photos, uninstalls apps with their leftovers, and manages the skills used by AI tools like Claude Code and Codex.",
    url: "https://cleanzo.echovic.com",
    author: "echoVic",
    mode: "web",
    image: "/showcase/cleanzo.jpg",
  },
  {
    name: "小海底 Little Reef",
    description:
      "A quiet, open-source focus timer for Mac. Every session you finish grows the seagrass a little and now and then a fish drops by; it also has a visitor log, 20 ambient sounds and a desktop widget.",
    url: "https://reef.echovic.com",
    author: "echoVic",
    mode: "web",
    image: "/showcase/little-reef.jpg",
  },
  {
    name: "MyTwig",
    description:
      "A Git client for macOS, Windows and Linux. A commit graph of every branch, diffs and a conflict editor, with GitHub pull requests, Jenkins pipelines and Jira issues built in, plus optional AI with your own provider or CLI: explain a commit or a change, search history in plain words, plan Git steps to run and undo, and write commit messages and pull request descriptions.",
    url: "https://mytwig.herosizy.dev",
    author: "HeroSizy",
    mode: "native",
    image: "/showcase/mytwig.svg",
  },
  {
    name: "Godwit",
    description:
      "An open-source, AI-native email client for macOS, Windows and Linux. It works with any IMAP/SMTP account, and its assistant reads, summarizes, translates and drafts mail with the model you choose; skills, plugins and MCP servers extend it.",
    url: "https://godwit.email",
    author: "baranwang",
    mode: "native",
    image: "/showcase/godwit.jpg",
  },
  {
    name: "Video-Downloader",
    description:
      "Native desktop video downloader built with Go, MyGo, yt-dlp, and FFmpeg. The application analyzes videos and playlists, lets users choose quality, audio, subtitles, and containers, manages downloads in a queue, converts local media, and stores download history.",
    url: "https://github.com/fazi-gondal/video-downloader-mygo",
    author: "Faizan",
    mode: "native",
    image: "/showcase/downloader.jpg",
  },
  {
    name: "Lumilo",
    description:
      "A menu-bar panel for local LLM engines on macOS. Start and stop Splash or llama.cpp, watch memory and speed, get models from Hugging Face and launch coding agents against the local server.",
    url: "https://github.com/kotuke/lumilo",
    author: "kotuke",
    mode: "native",
    image: "/showcase/lumilo.jpg",
  },
  {
    name: "Tomoru",
    description:
      "Live bilingual subtitles for anything you watch on macOS and Windows. It listens to what your computer plays, from raw anime and VTuber streams to games and meetings, and shows each line with its translation over any app.",
    url: "https://tomoru.acane.dev",
    author: "ACAne0320",
    mode: "native",
    image: "/showcase/tomoru.jpg",
  },
  {
    name: "Quaver Astra",
    description:
      "A lightweight, native-feeling QQ Music client. Team Quaver rebuilt their Electron app, Quaver Music, on MyGo, keeping its Go backend and playback engine.",
    url: "https://quaver.0w0.red",
    author: "Ne0W0r1d x Team Quaver",
    mode: "native",
    image: "/showcase/quaver-astra.webp",
  },
  {
    name: "Meiro",
    description:
      "YouTube Music, without the bloat. A native Material 3 Expressive player that takes about 100 MB, with no ads, synced lyrics, Last.fm scrobbling and colours that match each song.",
    url: "https://meiro.elianiva.com",
    author: "elianiva",
    mode: "native",
    image: "/showcase/meiro.jpg",
  },
  {
    name: "Repo Hopper",
    description:
      "Spotlight for your Git repositories. A global shortcut opens a search over every local repo, most recently changed first, and opens the chosen one in your editor, terminal or Finder.",
    url: "https://github.com/pteich/repo-hopper",
    author: "pteich",
    mode: "native",
    image: "/showcase/repo-hopper.jpg",
  },
  {
    name: "Universal S3 UI",
    description:
      "A lightweight storage browser for S3-compatible services, SFTP and FTP/FTPS. One connection manager with credentials in the system keychain, drag-and-drop uploads, temporary share links, and sync between any two connections.",
    url: "https://github.com/pteich/us3ui",
    author: "pteich",
    mode: "native",
    image: "/showcase/us3ui.jpg",
  },
  {
    name: "RcloneDrive",
    description:
      "An open-source tool that mounts cloud storage as local disks, a RaiDrive alternative. A single Go binary with pure native UI and no frontend toolchain, an embedded rclone engine with 70+ backends, mounted on Windows through WinFsp that installs silently, plus a system tray, launch at login and automatic updates from GitHub Releases.",
    url: "https://github.com/younland/RcloneDrive",
    author: "younland",
    mode: "native",
    image: "/showcase/rclonedrive.jpg",
  },
  {
    name: "ApiGo",
    description:
      "An API debugging tool in the shape of Hoppscotch, written in Go with MyGo native UI. Collections organize requests, REST and WebSocket traffic both send and render responses by type, and OpenAPI / Swagger documents import directly.",
    url: "https://github.com/younland/apigo",
    author: "younland",
    mode: "native",
    image: "/showcase/apigo.jpg",
  },
  {
    name: "Silverwing",
    description:
      "A task-oriented desktop workspace that keeps multiple repositories, branches, reference materials and review context together for each development task. Create tasks from Meegle requirements and prepare workspaces and task branches with Git worktrees, organize several repos in one task with built-in Git review, and carry AGENTS.md context into Codex, Cursor or an IDE. Windows x64 and macOS Apple silicon.",
    url: "https://github.com/javatoai/silverwing",
    author: "javatoai",
    mode: "native",
  },
  {
    name: "Nodra",
    description:
      "A responsive state management library for Go desktop apps, built around a concurrency-safe, versioned Store[T]: serialized concurrent updates that roll back on error, undo/redo with nested groups, codecs and storage layered for persistence and encryption, latest-wins async actions, and a read-only DevTools — with MyGo integration that redraws a window only when a subscribed value changes.",
    url: "https://nodra.yldm.ai",
    author: "houko",
    mode: "native",
    image: "/showcase/nodra.jpg",
  },
  {
    name: "NanoDoll",
    description:
      "A minimal desktop container built for web AI. Turn web AIs into standalone desktop apps: multi-pane tiling, browser login import, prompt library, and nearby file manager.",
    url: "https://moonvy.com/apps/nano-doll/",
    author: "yArna",
    mode: "native",
    image: "/showcase/nanodoll-cover.png",
  },
  {
    name: "Teebots",
    description:
      "A free desktop app for a small team of AI bots on your own Mac, Windows or Linux machine. Each bot uses your API key and model; chats, memory and files stay on the device.",
    url: "https://teebots.ai",
    author: "希嘉嘉",
    mode: "native",
    image: "/showcase/teebots.png",
  },
  {
    name: "映刻 Screenpin",
    description:
      "A Windows screenshot and screen recording utility that stays in the system tray. Global shortcuts capture a region, pin images above other windows, stitch scrolling screenshots, or record a region and trim the result before exporting.",
    url: "https://github.com/wiptgdxv/screenpin",
    author: "wiptgdxv",
    mode: "native",
    image: "/showcase/screenpin.png",
  },
  {
    name: "Sotto",
    description:
      "A lightweight, unobtrusive desktop translator built for Windows users and developers.",
    url: "https://github.com/doyuli/sotto",
    author: "doyuli",
    mode: "web",
    image: "/showcase/sotto.png",
  },
  {
    name: "TanGit",
    description:
      "An open-source Git desktop client built on Lazygit. Stage individual diff lines, browse history and branch graphs, resolve conflicts, and work across repositories on macOS, Windows and Linux with keyboard-driven native UI.",
    url: "https://github.com/FleetingEcho/TanGit",
    author: "FleetingEcho",
    mode: "native",
    image: "/showcase/tangit.jpg",
  },
  {
    name: "CurlGo",
    description:
      "An open-source, local-first API client for macOS and Windows, drawn with MyGo's native UI. Paste a curl command or import Postman v2.1 and Bruno collections, send HTTP, SSE and WebSocket requests, keep environments, global headers and a persistent cookie jar in one local workspace, and check responses with assertions and variable extraction; a CLI and agent skill run the same requests headlessly.",
    url: "https://github.com/echoVic/curlgo",
    author: "echoVic",
    mode: "native",
    image: "/showcase/curlgo.jpg",
  },
  {
    name: "GoRequest",
    description:
      "An open-source, local-first API client and documentation workspace with native UI. Debug HTTP and WebSocket requests, keep YAML requests and Markdown docs together in Git, import from cURL, Postman and OpenAPI, and run automated tests and local mocks.",
    url: "https://github.com/Flyinsky2004/GoRequest",
    author: "Flyinsky2004",
    mode: "native",
    image: "/showcase/gorequest.jpg",
  },
  {
    name: "Ferry",
    description:
      "A per-process proxy router for Windows, built on ProxyBridge, with ordered rules, SOCKS5 and HTTP proxy nodes, and live connections and logs.",
    url: "https://github.com/ayuayue/ferry",
    author: "ayuayue",
    mode: "web",
  },
  {
    name: "MyVNC",
    description:
      "A VNC and Microsoft Remote Desktop viewer for Windows and macOS, rebuilt from TigerVNC. Connecting to a Mac uses its built-in Screen Sharing, so the Mac installs nothing, with a high-performance mode streaming HEVC or H.264 at the source frame rate and file transfer both ways.",
    url: "https://github.com/si-mods/MyVNC",
    author: "si-mods",
    mode: "native",
    image: "/showcase/myvnc.jpg",
  },
  {
    name: "Golder",
    description:
      "An open-source AI coding agent in Go for macOS, Windows and Linux, with a native desktop app and a terminal/CLI. The desktop app brings project chats, file and Git review, an integrated terminal and browser together, with configurable approvals and experimental computer use.",
    url: "https://github.com/getan/golder",
    author: "getan",
    mode: "native",
    image: "/showcase/golder.jpg",
  },
  {
    name: "Vole",
    description:
      "A disk cleaner for macOS. A treemap of what fills your disk, safe cache cleanup you review item by item, app uninstalls that take the leftovers too, and live system status in the menu bar. Built on Mole.",
    url: "https://github.com/hellozhangran/vole",
    author: "hellozhangran",
    mode: "native",
    image: "/showcase/vole.jpg",
  },
  {
    name: "MiguWave",
    description:
      "A third-party Migu Music desktop client for macOS and Windows, drawn with MyGo's native UI. Discovery, charts, playlists and artist pages, scrolling lyrics, a full-screen vinyl stage with GPU-drawn effects, and Apple Music-style crossfade between tracks.",
    url: "https://github.com/horizon0514/MiguWave",
    author: "horizon0514",
    mode: "native",
    image: "/showcase/miguwave.jpg",
  },
  {
    name: "OpenBee",
    description:
      "An open-source AI team for macOS, Windows and Linux, with its server built in. Message Bee, the coordinator, and it hands each task to a worker running Claude Code, Codex or pi in its own folder, now, later or on a cron schedule; the same workers also answer in the browser and in chat apps like Telegram, Discord and Lark.",
    url: "https://github.com/theopenbee/openbee",
    author: "theopenbee",
    mode: "native",
    image: "/showcase/openbee.jpg",
  },
  {
    name: "Ferry Drive",
    description:
      "An open-source desktop client for Baidu Netdisk and Quark Drive on Apple Silicon Macs and Windows 10 or 11 (x64). Browse files, manage uploads and downloads, and open shared links.",
    url: "https://github.com/yjcdon/ferry-drive",
    author: "yjcdon",
    mode: "native",
    image: "/showcase/ferry-drive.png",
  },
  {
    name: "Snapover",
    description:
      "An open-source clipboard manager for macOS and Windows. Sync text, images, files and folders between computers over a direct, encrypted connection.",
    url: "https://github.com/2fa0/snapover",
    author: "2fa0",
    mode: "native",
    image: "/showcase/snapover.jpg",
  },
  {
    name: "Xylring",
    description:
      "An open-source native Git client for macOS and Windows. Browse commit graphs, review diffs, search history and stage individual hunks.",
    url: "https://github.com/2fa0/xylring",
    author: "2fa0",
    mode: "native",
    image: "/showcase/xylring.jpg",
  },
  {
    name: "Mojian 墨间",
    description:
      "A quiet, native plain-text editor for ordinary folders on macOS, Linux and Windows. Tabs hold split editors, image and PDF viewers and persistent terminals, with built-in Git diffs and remote projects over SSH; MyGo's native UI is the whole app, in one Go binary.",
    url: "https://mojian.ox0pine.cn",
    author: "ox0pine",
    mode: "native",
    image: "/showcase/mojian.jpg",
  },
  {
    name: "Paperly",
    description:
      "A native Markdown editor drawn with MyGo's native UI: the block under the caret shows its source while everything else stays rendered. File tree, full-text search, per-item list editing, and .md file associations on macOS and Windows, in one Go binary.",
    url: "https://github.com/jiangwanying/paperly",
    author: "jiangwanying",
    mode: "native",
    image: "/showcase/paperly.jpg",
  },
  {
    name: "Glide",
    description:
      "A lightweight native desktop word translation tool that supports macOS and Windows.",
    url: "https://github.com/anghunk/glide",
    author: "anghunk",
    mode: "native",
    image: "/showcase/glide.png",
  },
  {
    name: "Framely · 取景",
    description:
      "An open-source screenshot and image pinning tool for Windows, Linux and macOS, drawn with MyGo's native UI. Annotate captures, record a region, recognize text offline and translate it; Windows and Linux also support scrolling screenshots.",
    url: "https://github.com/wildalley/framely",
    author: "wildalley",
    mode: "native",
    image: "/showcase/framely.png",
  },
  {
    name: "MyGO Wallhaven",
    description:
      "An open-source Wallhaven wallpaper client built with Go and MyGO's native UI. Browse recommendations, search and filter wallpapers by resolution, aspect ratio and color, preview full-size images, save local favorites, manage resumable downloads, and set your desktop wallpaper.",
    url: "https://github.com/MaLuns/wallhaven",
    author: "MaLuns",
    mode: "native",
  },
  {
    name: "goFinder",
    description:
      "A keyboard-and-mouse file manager for macOS that brings Yazi's three columns and single-key actions to a native window. Filter as you type, even by pinyin initials, preview with Quick Look, split the window into two panes, and open an AI agent in the current folder.",
    url: "https://github.com/aoyamashou/goFinder",
    author: "aoyamashou",
    mode: "native",
    image: "/showcase/gofinder.jpg",
  },
  {
    name: "划词翻译",
    description:
      "A desktop translator for macOS and Windows. Select text and hold the right mouse button to translate it with a configured LLM, see the result in a popup and reuse locally cached translations.",
    url: "https://github.com/wojiukankan/translate",
    author: "wojiukankan",
    mode: "native",
    image: "/showcase/hua-ci-fan-yi.png",
  },
  {
    name: "Leafmark · 叶笺",
    description:
      "A lightweight WYSIWYG Markdown editor for macOS and Windows, drawn entirely with MyGo's native UI. Features multi-tab editing, folder workspaces, a leaf-vein outline, native TeX math and Mermaid diagram rendering, and HTML/PDF export.",
    url: "https://github.com/KevinXC5/leafmark",
    author: "KevinXC5",
    mode: "native",
    image: "/showcase/leafmark.jpg",
  },
]
