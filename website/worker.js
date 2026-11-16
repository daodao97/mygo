import { showcase } from "./showcase.ts"

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url)
    if (url.hostname === "mygo.egoist.dev") {
      url.protocol = "https:"
      url.hostname = "mygo.run"
      url.port = ""
      return Response.redirect(url.href, 308)
    }
    return (await showcase(request, ctx)) ?? env.ASSETS.fetch(request)
  },
}
