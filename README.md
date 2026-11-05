# Research projects

A collection of various research snippets, tests and experiments

It might be useful to keep this around for projects in the future or just because it fixes a few bugs or just because it's cool.

Structure:

- year/<experiment>
- inbox/<experiment>

The `inbox/` folder holds a tree for multiple ongoing experiments if needed. Otherwise it's just a drop space before archival.

Archival is year based for easy indexing. Older experiments moved into their appropriate years by the year of their first commit. There's no rule to archival, but I'm betting it may be regular around the january mark each year. I may set a distant reminder for claude.

Avoid if you're averse to LLM speak. Some of these research avenues are now done with claude, so they may read as a transcript of my inquery. In this case it's a tool for research, testing, benchmarking and whatever else I can think of dispatching a claude session for. These are my most time consuming workflows.

- Research: if you haven't noticed, the internet is full of ads
- Testing: the various testing feedback loops can be slow, need babysitting
- Benchmarking: stress testing your allocation practices is *tedious*

A coding assistant harness could just be these 3 things and could be immensly usable, but everything flows into "building code". Sigh.

- Research is done for implementation purposes, a need
- Testing is done for coverage, convention, structure
- Benchmarking is done for optimisations, allocation practice

I don't think research can be automated away, as it is a user feedback loop. Ideally I'd like to dispatch testing and benchmarking with automation and scheduling which just means including `claude` gets put into the CI/CD stack.

That's my 5 cents. Back to building stuff.

## Index

| Project                                                                                                                       | Year/month |
|-------------------------------------------------------------------------------------------------------------------------------|------------|
| [dpatch patches prefixing luasocket exports with `ls_*`, past a mod_magnet `buffer_init()` clash](2011/luasocket-2.0.2)       | 2011-03    |
| [Lua sharedance client, fetch only, keys under 255 bytes](2011/sharedance-lua)                                                | 2011-03    |
| [Steve Pomeroy's DTD-to-HTML Perl script, with ATTLIST formatting and `@todo` colouring](2011/dtd2html)                       | 2011-04    |
| [Start of a C++ stats daemon: MySQL client, INI parser, background forking](2013/wstatsd)                                     | 2013-01    |
| [Deferred leaf fetches over independent PHP data trees, folded into one multi-get](2013/php-resolve)                          | 2013-04    |
| [Photo similarity by RGB histogram, C++ over libjpeg](2015/photos_match)                                                      | 2015-05    |
| [Diffs `dpkg -l` across two hosts over ssh, prints the converging operations](2015/dpkg-sync)                                 | 2015-11    |
| [torrentz.eu RSS rewritten to torcache links, published through the Dropbox OAuth SDK](2016/defunct-eztv-dropbox)             | 2016-09    |
| [Polls that feed, matches INI title patterns, drops `.torrent` into a watch folder](2016/defunct-eztv-fetcher)                | 2016-09    |
| [Subversion commit mails with colorized diffs, path routing and PHP/XML lint gates](2016/defunct-mono-svn)                    | 2016-09    |
| [PDF to text and HTML: dockerised pdf2htmlEX and textract, then cheerio and juice](2016/pdfutils)                             | 2016-09    |
| [Book covers rendered from HTML at three fixed viewport sizes, webshot-cli under Docker](2016/docker-book-cover)              | 2016-12    |
| [nginx `proxy_cache` purge in Lua, single key and wildcard, reporting `X-Purged-Count`](2017/nginx-lua)                       | 2017-01    |
| [Five ways to pass services to Go handlers: factory, inject, struct, interface, interface2](2017/golang-dependencies)         | 2017-04    |
| [PHP against a TP-Link M5250 web interface: login, SMS, simpa.hr plan activation](2017/hacks-tp-link-m5250)                   | 2017-05    |
| [`defer r.Unlock()` against a plain call, recursion and `strings.Join`, under pprof and go-torch](2017/golang-benchmarking)   | 2017-06    |
| [wrk load test of a Go net/http endpoint writing SQLite rows](2018/docker-benchmark-io)                                       | 2018-06    |
| [Lists a Go file's interface declarations from a go/ast walk](2018/golang-parsing)                                            | 2018-06    |
| [In-place against allocating string word reversal, `testing.B` with ReportAllocs](2018/golang-allocations)                    | 2018-08    |
| [`go vet` copylocks on a value method of a sync.Mutex struct, minimal case](2018/golang-govetcopymutex)                       | 2018-08    |
| [Commit and prompt-log statistics, agent-written against developer-written repositories](2025/amp-vs-developer)               | 2025-11    |
| [System-wide exec tracing in Go over a CO-RE execve tracepoint, cilium/ebpf](2026/command-log-ebpf)                           | 2026-02    |
| [Go callback dispatch: a reflect tier against a closure-tree step JIT, moved to titpetric/gozero](2026/go-callback-overheads) | 2026-09    |

Index last updated on 2026-11-05.
