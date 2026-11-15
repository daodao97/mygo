import fs from "node:fs"
import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import { tanstackStart } from "@tanstack/react-start/plugin/vite"
import viteReact from "@vitejs/plugin-react"
import { defineConfig } from "vite"
import { docsDemo } from "./docs-demo.ts"
import { llmsDocs } from "./llms.ts"
import { showcase } from "./showcase.ts"

const repo = (file: string) => new URL(`../${file}`, import.meta.url)

// The pages of the repository's docs, plugins/fetch.md at /docs/plugins/fetch,
// and a directory's README at the directory, ui/README.md at /docs/ui. /docs
// itself, like every route without parameters, is found without being
// listed.
const docs = fs
  .readdirSync(repo("docs"), { recursive: true, encoding: "utf8" })
  .filter((file) => file.endsWith(".md") && file !== "README.md")
  .map((file) => `/docs/${file.slice(0, -3).split(path.sep).join("/").replace(/\/README$/, "")}`)
  // The docs in Chinese, under /zh, have the same pages.
  .flatMap((page) => [page, `/zh${page}`])

const version = /const Version = "([^"]+)"/.exec(fs.readFileSync(repo("mygo.go"), "utf8"))?.[1]
const goVersion = /^go (\S+)/m.exec(fs.readFileSync(repo("go.mod"), "utf8"))?.[1]

export default defineConfig({
  define: {
    __MYGO_VERSION__: JSON.stringify(version),
    __GO_VERSION__: JSON.stringify(goVersion),
  },
  resolve: { tsconfigPaths: true },
  plugins: [
    // /api/showcase, which the Worker serves in production, during `vite dev`.
    {
      name: "showcase",
      configureServer(server) {
        server.middlewares.use(async (req, res, next) => {
          const response = await showcase(new Request(`http://localhost${req.url}`, { method: req.method }), { waitUntil() {} })
          if (!response) return next()
          res.statusCode = response.status
          response.headers.forEach((value, name) => res.setHeader(name, value))
          res.end(Buffer.from(await response.arrayBuffer()))
        })
      },
    },
    llmsDocs(path.resolve(import.meta.dirname, "..")),
    docsDemo(path.resolve(import.meta.dirname, "..")),
    tailwindcss(),
    tanstackStart({
      // A static site: every page is rendered to HTML at build time.
      // /docs-demo is not a page but files of docs-demo.ts, which previews
      // link to.
      prerender: { enabled: true, crawlLinks: true, filter: ({ path }) => !path.includes("#") && !path.startsWith("/docs-demo") },
      pages: [
        // The pages in Chinese are reached from /zh, the others from /.
        { path: "/zh" },
        ...docs.map((path) => ({ path })),
        { path: "/docs/search.json" },
        { path: "/zh/docs/search.json" },
        // Served by Cloudflare for any path that has no page.
        { path: "/404", prerender: { enabled: true, outputPath: "/404.html" } },
      ],
    }),
    viteReact(),
  ],
})
