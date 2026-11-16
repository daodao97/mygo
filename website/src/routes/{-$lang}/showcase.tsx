import { createFileRoute } from "@tanstack/react-router"
import { ArrowUpRightIcon } from "lucide-react"
import * as React from "react"

import type { ShowcaseApp } from "../../../showcase.ts"

import { translate, useT } from "@/lib/i18n"
import { langOfMatches } from "@/lib/locale"

const title = "Showcase · MyGo"
const description = "Apps built with MyGo by the community, with native UI or a web frontend."
// The apps live in this repository; the Worker serves them from Cloudflare's
// cache (showcase.ts), so a merged entry shows up without a new build.
const repo = "https://github.com/egoist/mygo-showcase"
const dataFile = `${repo}/blob/main/showcase.yaml`
const addYours = `${repo}#add-your-app`

const modes = { native: "Native UI", web: "Web frontend" } as const

export const Route = createFileRoute("/{-$lang}/showcase")({
  head: ({ matches }) => {
    const lang = langOfMatches(matches)
    return {
      meta: [
        { title: translate(lang, title) },
        { name: "description", content: translate(lang, description) },
        { property: "og:title", content: translate(lang, title) },
        { property: "og:description", content: translate(lang, description) },
      ],
    }
  },
  component: Showcase,
})

type State = { apps: ShowcaseApp[] } | { error: true } | null

function useShowcase(): State {
  const [state, setState] = React.useState<State>(null)
  React.useEffect(() => {
    const controller = new AbortController()
    fetch("/api/showcase.json", { signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`${response.status}`)
        return response.json() as Promise<ShowcaseApp[]>
      })
      .then((apps) => setState({ apps }))
      .catch(() => controller.signal.aborted || setState({ error: true }))
    return () => controller.abort()
  }, [])
  return state
}

function Showcase() {
  const t = useT()
  const state = useShowcase()
  return (
    <main className="flex-1">
      <section className="border-b px-4 pt-16 pb-12 sm:px-10 md:pt-20">
        <p className="label">{t("Showcase")}</p>
        <h1 className="mt-5 text-3xl leading-[1.1] font-semibold tracking-[-0.035em] sm:text-4xl">{t("Built with MyGo.")}</h1>
        <p className="mt-5 max-w-2xl text-base leading-7 text-muted-foreground">
          {t("Apps the community has made with MyGo, drawing native UI or showing a web frontend.")}
        </p>
        <p className="mt-6 text-sm">
          <a href={addYours} className="font-medium underline decoration-gopher/45 underline-offset-[5px] hover:decoration-gopher">
            {t("Add your app")}
          </a>
        </p>
      </section>

      {state && "error" in state ? (
        <p className="border-b px-4 py-10 text-sm text-muted-foreground sm:px-10">
          {t("The apps could not be loaded. See them in")}{" "}
          <a href={dataFile} className="font-mono text-[13px] text-foreground underline underline-offset-4">
            egoist/mygo-showcase
          </a>
        </p>
      ) : (
        <div className="-mr-px grid sm:grid-cols-2 lg:grid-cols-3" aria-busy={!state}>
          {state ? state.apps.map((app) => <Card key={app.url} app={app} />) : Array.from({ length: 6 }, (_, i) => <Placeholder key={i} />)}
        </div>
      )}

      <section className="px-4 py-10 sm:px-10">
        <h2 className="text-xl font-semibold tracking-tight">{t("Add your app")}</h2>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
          {t("Built something with MyGo? Open a pull request that adds it to")}{" "}
          <a href={dataFile} className="font-mono text-[13px] text-foreground underline underline-offset-4">
            showcase.yaml
          </a>{" "}
          {t("in egoist/mygo-showcase")}
          {t(", with its name, a short description, a link, and whether it draws native UI or shows a web frontend. A screenshot is optional.")}
        </p>
      </section>
    </main>
  )
}

function Placeholder() {
  return (
    <div className="flex flex-col border-r border-b" aria-hidden>
      <div className="aspect-[16/10] w-full animate-pulse border-b bg-muted" />
      <div className="flex flex-col gap-3 px-4 py-5 sm:px-6">
        <div className="h-5 w-1/3 animate-pulse rounded bg-muted" />
        <div className="h-4 w-full animate-pulse rounded bg-muted" />
        <div className="h-4 w-2/3 animate-pulse rounded bg-muted" />
      </div>
    </div>
  )
}

function Card({ app }: { app: ShowcaseApp }) {
  const t = useT()
  return (
    <article className="group relative flex min-w-0 flex-col border-r border-b">
      {app.image && <img src={app.image} alt="" loading="lazy" className="aspect-[16/10] w-full border-b object-cover object-top" />}
      <div className="flex flex-1 flex-col gap-3 px-4 py-5 sm:px-6">
        <header className="flex items-start justify-between gap-4">
          <h2 className="min-w-0 font-semibold tracking-tight">
            <a href={app.url} className="after:absolute after:inset-0 group-hover:underline group-hover:underline-offset-4">
              {app.name}
            </a>
          </h2>
          <ArrowUpRightIcon className="size-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground" />
        </header>
        <p className="text-sm leading-6 text-muted-foreground">{app.description}</p>
        {(app.mode || app.author) && (
          <p className="label mt-auto flex flex-wrap gap-x-4 gap-y-1 pt-1">
            {app.mode && <span>{t(modes[app.mode])}</span>}
            {app.author && <span>{t("By {author}", { author: app.author })}</span>}
          </p>
        )}
      </div>
    </article>
  )
}
