import { expect, test } from "bun:test"
import worker from "./worker.js"

test("permanently redirects the old domain, preserving the path and query", async () => {
  for (const path of ["/", "/zh/docs/app?from=old&lang=zh", "/assets/app.js", "/docs/%E4%B8%AD%E6%96%87?q=a%2Fb"]) {
    const response = await worker.fetch(new Request(`https://mygo.egoist.dev${path}`), {
      ASSETS: { fetch() { throw new Error("Old-domain requests must not serve assets") } },
    })
    expect(response.status).toBe(308)
    expect(response.headers.get("Location")).toBe(`https://mygo.run${path}`)
  }
})

test("redirects HTTP requests to HTTPS on the new domain", async () => {
  const response = await worker.fetch(new Request("http://mygo.egoist.dev/docs?query=1"), {})
  expect(response.headers.get("Location")).toBe("https://mygo.run/docs?query=1")
})

test("serves canonical, preview and local requests without redirecting", async () => {
  for (const host of ["mygo.run", "preview-mygo.example.workers.dev", "localhost:8787"]) {
    const request = new Request(`https://${host}/zh/docs/app?query=1`)
    const asset = new Response("page", { status: 200 })
    const response = await worker.fetch(request, {
      ASSETS: { fetch(received) { expect(received).toBe(request); return asset } },
    })
    expect(response).toBe(asset)
  }
})

// A stand-in for Cloudflare's caches.default, and fetch answering for GitHub.
function fakes(files) {
  const store = new Map()
  const fetched = []
  globalThis.caches = {
    default: {
      async match(request) { return store.get(request.url)?.clone() },
      async put(request, response) { store.set(request.url, response) },
    },
  }
  const realFetch = globalThis.fetch
  globalThis.fetch = async (url) => {
    fetched.push(String(url))
    const file = files[String(url).replace("https://raw.githubusercontent.com/egoist/mygo-showcase/main/", "")]
    if (file instanceof Error) throw file
    return file === undefined ? new Response("404", { status: 404 }) : new Response(file)
  }
  const pending = []
  const ctx = { waitUntil(p) { pending.push(p) } }
  const get = async (path) => {
    const response = await worker.fetch(new Request(`https://mygo.run${path}`), { ASSETS: { fetch() { throw new Error("not an asset") } } }, ctx)
    await Promise.all(pending)
    return response
  }
  return { get, fetched, files, store, restore() { globalThis.fetch = realFetch; delete globalThis.caches } }
}

const yaml = `
- name: App
  description: >-
    Does
    things.
  url: https://example.com/app
  author: someone
  mode: native
  image: images/app.jpg
- name: No URL
  description: Left out.
- name: Odd
  description: Its mode and image are dropped.
  url: https://example.com/odd
  mode: cli
  image: ../secret.jpg
`

test("serves showcase.yaml as JSON, checked, with screenshots on this origin", async () => {
  const f = fakes({ "showcase.yaml": yaml })
  try {
    const response = await f.get("/api/showcase.json")
    expect(response.headers.get("Content-Type")).toBe("application/json; charset=utf-8")
    expect(response.headers.get("Cache-Control")).toBe("public, max-age=300")
    expect(await response.json()).toEqual([
      { name: "App", description: "Does things.", url: "https://example.com/app", author: "someone", mode: "native", image: "/api/showcase/images/app.jpg" },
      { name: "Odd", description: "Its mode and image are dropped.", url: "https://example.com/odd" },
    ])
  } finally {
    f.restore()
  }
})

test("answers from the cache, and from the last good copy while GitHub fails", async () => {
  const f = fakes({ "showcase.yaml": yaml })
  try {
    await f.get("/api/showcase.json")
    await f.get("/api/showcase.json")
    expect(f.fetched.length).toBe(1)

    f.store.delete("https://mygo.run/api/showcase.json") // expired
    f.files["showcase.yaml"] = new Error("offline")
    const response = await f.get("/api/showcase.json")
    expect(response.status).toBe(200)
    expect((await response.json())[0].name).toBe("App")

    f.store.clear()
    expect((await f.get("/api/showcase.json")).status).toBe(502)
  } finally {
    f.restore()
  }
})

test("serves screenshots with their type, and an SVG that can't run scripts", async () => {
  const f = fakes({ "images/app.svg": "<svg/>" })
  try {
    const response = await f.get("/api/showcase/images/app.svg")
    expect(response.headers.get("Content-Type")).toBe("image/svg+xml")
    expect(response.headers.get("Content-Security-Policy")).toContain("sandbox")
    expect(response.headers.get("Cache-Control")).toBe("public, max-age=86400")
    expect((await f.get("/api/showcase/images/missing.png")).status).toBe(404)
    expect((await f.get("/api/showcase/images/..%2Fshowcase.yaml")).status).toBe(404)
    expect((await f.get("/api/showcase/other")).status).toBe(404)
    expect(f.fetched).toEqual([
      "https://raw.githubusercontent.com/egoist/mygo-showcase/main/images/app.svg",
      "https://raw.githubusercontent.com/egoist/mygo-showcase/main/images/missing.png",
    ])
  } finally {
    f.restore()
  }
})
