# MyGo website

The home page and, at `/docs`, the guides of [`../docs`](../docs), built with
TanStack Start and shadcn/ui. Every page is prerendered at build time, and
Cloudflare Workers serves the result as static assets, with a small Worker
that redirects requests from the old domain before serving assets.

Production deploys attach the Worker to `mygo.run` and `mygo.egoist.dev`
through the custom domains in `wrangler.jsonc`. `worker.js` permanently
redirects `mygo.egoist.dev` to `https://mygo.run`, preserving the path and
query string. Requests to `mygo.run`, preview URLs and localhost serve assets
normally. Preview deployments do not change the production domains.

```sh
bun install                    # at the root of the repository
bun run --cwd website dev      # http://localhost:3000
bun run --cwd website build    # dist/client
bun run --cwd website preview  # dist/client, served by wrangler as in production
bun run --cwd website deploy   # build, then wrangler deploy
```

The apps of `/showcase` live in
[egoist/mygo-showcase](https://github.com/egoist/mygo-showcase), so adding
one needs no new build. The Worker (`showcase.ts`) serves its `showcase.yaml`
as `/api/showcase.json` and its screenshots under `/api/showcase/images/`,
from Cloudflare's cache (5 minutes for the list, a day for screenshots), and
keeps the last good copy to serve while GitHub fails. `vite dev` serves them
through the same code; the page fetches the list once it opens.

Pages of the docs are rendered from the markdown when the site is built:
the order of the sidebar comes from the lists of `docs/README.md`, pages in
directories keep their path (`plugins/fetch.md` is `/docs/plugins/fetch`),
and links between pages, such as `bindings.md#events`, become links of the
site. Lists of pages, `- [Title](page.md): what it covers`, become cards.

The same build generates `/llms.txt`, an index of Markdown documentation,
and `/llms-full.txt`, all pages combined. Append `.md` to a docs page's URL
for its Markdown version: `/docs.md`, `/docs/ui.md`, or
`/docs/plugins/fetch.md`. These downloads are also served during development.
Links between Markdown pages point to their published URLs; links to other
repository files point to GitHub. Docs pages advertise their Markdown version
and the index through HTML link elements.

Maintainer-only instructions can stay in the repository docs between
`<!-- repository-only:start -->` and `<!-- repository-only:end -->` on
separate lines. The website omits those blocks from the page, table of
contents, search index and Markdown downloads; GitHub still renders their contents.

A comment `<!-- preview button 140 -->` in a page becomes a window of
macOS, as high as it says, running that demo of `website/docs-demo` (a Go
app of native UI compiled to WebAssembly), and `<!-- app mayoi 480 -->`
runs an example app, one of `apps` in `docs-demo.ts`; a button of its title
bar opens it in a tab of its own.

The site is in English at its paths, and in Chinese under `/zh`: every page
is prerendered in both, `/docs/plugins/fetch` and `/zh/docs/plugins/fetch`.
A browser whose languages ask for Chinese first, and no choice of English in
the header, is sent from an English page to its page under `/zh` before it
paints (`localeScript`); the header's picker keeps the choice. The site's
text is written in English, and `src/lib/zh.ts` holds the Chinese of each
English text, which `useT()` looks up: a text without one stays in English.
The routes live in `src/routes/{-$lang}`, a path segment that is `zh` under
`/zh` and absent at the English paths, and `vite.config.ts` lists `/zh`
among the pages to prerender, the root of the Chinese site, which nothing
else links to.

The docs are translated too: `../docs-zh` holds the Chinese of `../docs`,
page for page, except `architecture.md`, which is not translated, and
`/zh/docs/...` renders from it, its sidebar and its search from
`../docs-zh/README.md` and `/zh/docs/search.json`. A page without a
translation falls back to the English page, with its links under `/zh`.
`../docs-zh/TRANSLATION.txt` is the translators' guide. The Markdown
downloads include the Chinese pages, `/zh/docs/plugins/fetch.md`.

The 404 page, prerendered as `404.html`, is English at any path that has no
page: one HTML serves both languages, and a page in Chinese would differ
from it as it hydrates.

Native UI's docs, in `docs/ui`, have a sidebar of their own, from the lists
of `docs/ui/README.md` (the page at `/docs/ui`), and the header's UI link;
the docs' sidebar leaves them out. A directory's `README.md` is the page of
the directory.
