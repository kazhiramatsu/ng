# Keystroke latency

2026-11-13, ng v0.5.0.

## Method

`--trace` records, for each keystroke, when the window system delivered
the key (SDL's event timestamp), when Neovim's answering flush arrived,
and when the frame was presented (package `latency`). It covers ng and
Neovim, not the compositor or the display; compare whole-system latency
with a terminal using an external tool such as Typometer (X11).

## A measurement bug

The first run reported p90 97 ms and max 183 ms. The trace showed that
the slow samples were not slow: `s` at 4318 ms had its sample logged at
4381 ms, but reported 182 ms. The recorder kept the first key that had
not been answered yet; a key that changes nothing on screen (the start
of a mapping, an operator waiting for its motion) stayed pending, and
the next key's frame was measured from it, adding the time between the
two keystrokes. It now measures from the last key before the flush, and
each trace line names its key.

## Result

176 keystrokes of ordinary editing (Manjaro, Neovim 0.13 nightly, the
author's config):

| Part | Median (ms) | p90 (ms) | Max (ms) |
| --- | --- | --- | --- |
| Total | 7.95 | 8.46 | 121.90 |
| SDL queue | 0.14 | 0.20 | 0.82 |
| Neovim | 0.72 | 1.00 | 106.18 |
| Frame | 6.99 | 7.40 | 15.47 |

The three samples over 20 ms were Neovim's own work (`:`, `<CR>`, a
mapping).

## The 4 ms wait

Most of the frame part is the wait for Neovim to finish a burst of
flushes (package `frame`: present after 4 ms without a flush, at most
16 ms). Presenting earlier showed intermediate screens, such as a blank
command line during completion. 2 ms / 8 ms was tried first and still
flickered; 4 / 16 came from flush intervals measured in traces. Cutting
the wait for keystrokes only is possible but risks that flicker; not
done. Since then ng presents at once when Neovim says it is idle (below).

## Presenting the final flush at once

2026-11-15, Neovim 0.12.6.

Where do the bursts come from? Traces of everyday commands, typed over
one RPC channel as ng types them (a script writing `nvim_input` to
`--listen`'s socket), show:

- **Normal mode:** with 'showcmd' (on by default) a command flushes
  twice: first the key shown in the corner, then the result 0.02-3 ms
  later.
- **Search with 'incsearch':** the typed character, then the matches
  2-4 ms later.
- **Completion:** "-- Searching..." or a blank command line, then the
  result 0.1-3 ms later.

So the 4 ms wait mostly waited for flushes that were already there.
Neovim triggers SafeState when it is about to wait for input, and its
last flush comes right after. ng's bootstrap sends `ng_idle` from
SafeState (`nvim.IdleNotification`), and ng presents the next flush at
once (package `frame`). There is no SafeState while completion is
active, an operator or a mapping is pending, or keys are queued; then
ng waits as before.

Two traps:

- Keys fed from a Lua timer or with `nvim --remote-send` trigger an
  extra SafeState before the key is read. Only `nvim_input` on one open
  channel, as ng sends it, shows the real order.
- A timer that changes nothing still sends `ng_idle`, with no flush
  after it. ng ignores an `ng_idle` from before its last write to Neovim
  (`rpc.Client.LastWrite`) or more than `MaxDelay` before the flush.

Two known limits:

- An `ng_idle` from an async wake-up that redraws nothing has no flush
  after it. If it arrives just after ng writes a key, that key's first
  flush can be taken for final.
- Async redraws, such as `:terminal` output or timers, arrive as
  `ng_idle` and a flush. They are presented at once, without the 4 to
  16 ms merging.

`TestIdleComesBeforeTheFinalFlush` checks the order against Neovim
stable and nightly in the daily CI run.

Results on Linux (Ubuntu 26.04 in VirtualBox with 3D acceleration:
OpenGL on Mesa's SVGA3D, on Xorg):

| Median (ms) | before | after |
| --- | --- | --- |
| Real X key events (xdotool, 95 keys, 120x40): key event to present | 5.5 | 1.1 |
| Everyday commands over RPC (120x40): last flush to present | 5.2 | 0.7 |
| Scrolling a line every 8 ms (216x70): flush to present | 5.2 | 0.7 |
| `<C-f>` / `<C-b>` every 16 ms (216x70): flush to present | 5.2 | 0.7 |

## Moving rows by renderer

A scroll repainted every row of its region. Rows are painted
independently, so now the painted rows move with their cells
(`screen.TakeChanges`, `paint.Scroll`). The GUI copies them through a
second texture, since SDL cannot draw a texture onto itself.

Scrolling a line every 8 ms, 216x70 (1944x1260 px, 800 flushes), with
rows moved or repainted. Median flush to present, and ng's CPU time
(user and system) or the frames presented:

| Renderer | moved | repainted |
| --- | --- | --- |
| OpenGL on a virtual GPU | 0.16 ms, 0.51 s | 0.36 ms, 0.68 s |
| SDL software | 6.5 ms, 540 frames | 7.5 ms, 450 frames |
| OpenGL on llvmpipe | 13 ms, 230 frames | 5.3 ms, 680 frames |
| Vulkan on lavapipe | 60 ms, 60 frames | 9.2 ms, 370 frames |

The machine is Ubuntu 26.04 in VirtualBox 7.2 on an Apple M5 Mac. The
virtual GPU is VirtualBox's 3D acceleration, on Xorg: Mesa's SVGA3D
driver hands OpenGL to the host's GPU, so ng's CPU time leaves that work
out. The other rows run on Xvfb, where OpenGL and Vulkan are Mesa's CPU
implementations, as in virtual machines without 3D acceleration.

On a GPU, moving halves the time to present. With SDL's software
renderer it is a little faster. On llvmpipe and lavapipe the two
full-window copies cost more than painting the rows, and ng fell behind
Neovim. So ng asks the renderer for its device (`ng_renderer_device`:
`GL_RENDERER` or the Vulkan device name) and repaints when it is
llvmpipe, softpipe or SwiftShader (`onCPU` in package gui; the trace
says which).

## New characters

Each character missing from 'guifont' was looked up with `FcFontSort`,
which scores and sorts every installed font. A screen of Japanese text
can have hundreds of characters ng has not drawn yet, and the cost grows
with the number of fonts.

ng now sorts once per style and walks that list for each new character
(package font, `ng_fallbacks_t`), as Pango does. `BenchmarkNewCJKGlyphs`
(lookup and rasterizing, per character) on Ubuntu 26.04:

| Fonts installed | before | after |
| --- | --- | --- |
| 11 (DejaVu, IPAGothic) | 43 µs | 2.7 µs |
| 313 (with Noto) | 142 µs | 8.1 µs |

The font picked can change. Of 66 test characters, with Noto and a
Japanese locale, six symbols (⚠ ∑ ≤ █ ▒ ™) now come from DejaVu Sans
Mono instead of Noto Sans CJK; with an English locale, Hebrew comes from
Noto Sans Hebrew instead of DejaVu Sans. Kanji, kana, Hangul and emoji
did not change.
