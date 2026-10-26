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
    mode: "web",
  },
]
