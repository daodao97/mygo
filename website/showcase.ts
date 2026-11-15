// The apps of /showcase, which live in egoist/mygo-showcase so that adding one
// needs no new build of the website. The Worker serves its showcase.yaml as
// /api/showcase.json and its screenshots under /api/showcase/images/, from
// Cloudflare's cache: the page reads them from the site's own origin, which
// readers reach where raw.githubusercontent.com is blocked.
import { parse } from "yaml"

export const source = "https://raw.githubusercontent.com/egoist/mygo-showcase/main/"

/** How long a copy is served before GitHub is asked again, in seconds. */
const fresh = { list: 300, image: 86400 }
/** How long the last good copy stays to serve while GitHub fails. */
const lastGood = 30 * 86400

const imagePath = /^\/api\/showcase\/images\/([a-z0-9][a-z0-9._-]*\.(jpe?g|png|webp|svg))$/
const imageTypes: Record<string, string> = { jpg: "image/jpeg", jpeg: "image/jpeg", png: "image/png", webp: "image/webp", svg: "image/svg+xml" }

/** An app of the showcase, as /api/showcase.json lists it. */
export interface ShowcaseApp {
  name: string
  description: string
  url: string
  author?: string
  mode?: "native" | "web"
  /** A path on this origin, under /api/showcase/images/. */
  image?: string
}

interface Context {
  waitUntil(promise: Promise<unknown>): void
}

/** The response to a request of /api/showcase.json or /api/showcase/images/*, or null for any other path. */
export async function showcase(request: Request, ctx: Context): Promise<Response | null> {
  const url = new URL(request.url)
  if (url.pathname === "/api/showcase.json") return cached(request, ctx, fresh.list, list)
  const image = imagePath.exec(url.pathname)
  if (image) return cached(request, ctx, fresh.image, () => screenshot(image[1]!, imageTypes[image[2]!]!))
  if (url.pathname.startsWith("/api/showcase")) return new Response("Not found", { status: 404 })
  return null
}

/** The apps of showcase.yaml, each checked, with screenshots served by this Worker. */
async function list(): Promise<Response> {
  const response = await fetch(`${source}showcase.yaml`)
  if (!response.ok) throw new Error(`showcase.yaml: ${response.status}`)
  const apps = parse(await response.text())
  if (!Array.isArray(apps)) throw new Error("showcase.yaml is not a list")
  const out: ShowcaseApp[] = []
  for (const app of apps) {
    if (!app || typeof app !== "object") continue
    const { name, description, url, author, mode, image } = app
    if (![name, description, url].every((v) => typeof v === "string" && v.trim())) continue
    if (!url.startsWith("https://")) continue
    const entry: ShowcaseApp = { name, description, url }
    if (typeof author === "string" && author) entry.author = author
    if (mode === "native" || mode === "web") entry.mode = mode
    if (typeof image === "string" && imagePath.test(`/api/showcase/${image}`)) entry.image = `/api/showcase/${image}`
    out.push(entry)
  }
  return new Response(JSON.stringify(out), { headers: { "Content-Type": "application/json; charset=utf-8" } })
}

/** A screenshot of images/, which a contributor's SVG can't turn into a page of this origin. */
async function screenshot(file: string, type: string): Promise<Response> {
  const response = await fetch(`${source}images/${file}`)
  if (response.status === 404) return new Response("Not found", { status: 404 })
  if (!response.ok) throw new Error(`images/${file}: ${response.status}`)
  return new Response(response.body, {
    headers: {
      "Content-Type": type,
      "Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; sandbox",
      "X-Content-Type-Options": "nosniff",
    },
  })
}

/**
 * The cached response to a GET, else what make returns, cached for maxAge
 * seconds. While make fails, the last good copy is served, for lastGood.
 */
async function cached(request: Request, ctx: Context, maxAge: number, make: () => Promise<Response>): Promise<Response> {
  if (request.method !== "GET" && request.method !== "HEAD") return new Response("Method not allowed", { status: 405, headers: { Allow: "GET, HEAD" } })
  // caches.default is Cloudflare's; there is none under Node or Bun.
  const cache = (globalThis as { caches?: { default?: Cache } }).caches?.default
  const key = new Request(new URL(request.url).origin + new URL(request.url).pathname)
  const backup = new Request(`${key.url}?last-good`)
  const hit = await cache?.match(key)
  if (hit) return hit

  let response: Response
  try {
    response = await make()
  } catch (error) {
    const stale = await cache?.match(backup)
    if (stale) return stale
    console.error(error)
    return new Response("The showcase could not be fetched from GitHub", { status: 502 })
  }
  if (!response.ok) return response

  response = new Response(response.body, response)
  response.headers.set("Cache-Control", `public, max-age=${maxAge}`)
  if (cache) {
    const copy = response.clone()
    const kept = new Response(response.clone().body, response)
    kept.headers.set("Cache-Control", `public, max-age=${lastGood}`)
    ctx.waitUntil(Promise.all([cache.put(key, copy), cache.put(backup, kept)]))
  }
  return response
}
