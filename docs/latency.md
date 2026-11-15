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

2026-11-14, Neovim 0.12.6, measured on macOS 26 (Apple M5, ng's macOS port
with the Metal renderer); nothing here depends on the platform.

Where do the bursts come from? Traces of everyday commands, typed over
one RPC channel as ng types them (a script writing `nvim_input`
notifications to `--listen`'s socket), grouped by keystroke:

- **Normal mode:** almost every command flushes twice. With 'showcmd' (on
  by default) `normal_cmd` shows the key in the corner and calls
  `ui_flush()` before running the command; the second flush clears it
  and has the result. The gap is the command's own time: 0.02-0.4 ms for
  `j` or `w`, 1-3 ms for `<C-d>`, `o` or `<C-w>v`.
- **Search with 'incsearch':** the typed character with `busy_start`,
  then the matches 2-4 ms later.
- **Completion** (`<C-n>`, `<Tab>` on the command line): "-- Searching..."
  or a blanked command line, then the result 0.1-3 ms later.

So the 4 ms wait mostly waited for flushes that were already there after
0.2 ms. Neovim triggers SafeState when nothing is pending and it is
about to wait for input, and its last flush comes right after (it
flushes before blocking). ng's bootstrap sends `ng_idle` from SafeState
(`nvim.IdleNotification`), and the flush after it is presented at once
(package `frame`). In the traces every burst with SafeState ended with
`idle flush`; there was never a flush after it. SafeState is not
triggered while completion is active, an operator or a mapping is
pending, or keys are still queued: there ng waits as before.

Two traps on the way:

- **Test drivers make extra SafeStates.** Keys fed from a Lua timer or
  with `nvim --remote-send` (a new channel per key) trigger SafeState
  for the timer or the channel itself, before the key is read, and the
  showcmd flush then looked final. Only a driver that writes
  `nvim_input` on one open channel, as ng does, gives the real order.
- **A stale `ng_idle`.** A timer that changes nothing still sends one,
  with no flush after it. ng ignores an `ng_idle` that arrived before its
  last write to Neovim (`rpc.Client.LastWrite`) or more than `MaxDelay`
  before the flush, so the next keystroke's showcmd flush is not taken
  for final.

`TestIdleComesBeforeTheFinalFlush` checks the order against Neovim stable
and nightly every day.

Results (`ng --trace`, the last flush of each burst to its present):

| | before | after |
| --- | --- | --- |
| Everyday commands, 120x40, median / p90 (ms) | 5.00 / 5.90 | 0.41 / 1.38 |
| Scrolling a line every 8 ms, 216x70 (3024x1836 px) | 5.26 / 5.38 | 0.20 / 0.26 |
| `<C-f>` / `<C-b>` every 16 ms, 216x70 | 5.21 / 5.55 | 0.38 / 0.68 |

With real keystrokes (key events posted to ng's process, 91 keys of
everyday editing per run, two runs each, 120x40), ng's own summary:

| `--trace` summary, median (p90), ms | before | after |
| --- | --- | --- |
| total: key event to present | 5.57 (6.8-7.3) | 1.02 (2.5-2.8) |
| frame: flush to present | 4.83 (5.1) | 0.25 (0.6) |
| nvim | 0.43 | 0.44 |

The scrolling numbers also include moving the painted rows instead of
repainting them (`paint.Scroll`, `screen.TakeChanges`): one line of
scrolling repaints one row, not 68. ng's CPU time for the same scrolling
went from 0.93 to 0.57 s (user) with this and the faster painting
together; "Moving rows by renderer" below has the share of moving rows.

## Moving rows by renderer

Moving painted rows copies them through a second texture (SDL cannot
draw a texture onto itself). Scrolling one line every 8 ms, 216x70 cells
unless said otherwise, with rows moved or, with that turned off,
repainted (median flush to present; ng's CPU time, user and system, or
the frames presented):

| Renderer | moved | repainted |
| --- | --- | --- |
| Metal (macOS, Apple M5, 3024x1836 px) | 0.22 ms, 1.02 s | 0.42 ms, 1.14 s |
| OpenGL on a virtual GPU (VirtualBox), 1944x1260 px | 0.18 ms, 0.55 s | 0.37 ms, 0.75 s |
| SDL software, macOS, 120x40 cells, 1680x1080 px | 1.74 ms, 2.23 s | 2.08 ms, 2.34 s |
| SDL software, macOS, 3024x1836 px | 4.86 ms, 5.30 s | 4.65 ms, 5.07 s |
| SDL software, Linux VM, 1944x1260 px | 10.7 ms, 336 frames | 15.3 ms, 233 frames |
| OpenGL on llvmpipe, Linux VM | 552 ms, 15 frames | 19 ms, 174 frames |
| Vulkan on lavapipe, Linux VM | 940 ms, 9 frames | 300 ms, 18 frames |

(800 flushes each; the Linux VM is Ubuntu 26.04 in UTM on the same Mac,
on Xvfb, where OpenGL and Vulkan are Mesa's CPU implementations, as in
virtual machines without 3D acceleration. Shougo/ng main presented 88
frames there on OpenGL, 39 ms late. The virtual GPU is Ubuntu 26.04 in
VirtualBox 7.2 with 3D acceleration, on Xorg: Mesa's SVGA3D driver hands
OpenGL to VirtualBox, which draws with the Mac's GPU, so ng's CPU time
there leaves that work out. Moved or repainted, 797 to 800 of the 800
flushes were presented, and OpenGL ES 2 gave the same times; Shougo/ng
main presented them 5.15 ms late, the 4 ms wait.)

On a GPU, moving halves the time to present. With SDL's own software
renderer it is about even. With OpenGL or Vulkan running on the CPU the
two full-window copies, and the render target switches between them,
cost far more than painting the rows: ng fell behind Neovim and showed
a frame every half second. ng asks the renderer for its device
(`ng_renderer_device`: OpenGL's `GL_RENDERER`, the Vulkan device's name)
and repaints the rows when it is llvmpipe, softpipe or SwiftShader
(`onCPU` in package gui; the trace says which).

Key event to present on the same Linux VM, real X key events from
xdotool, 95 keystrokes: median 10.6 to 6.0 ms with the software renderer
and 7.8 to 3.2 ms with OpenGL on llvmpipe, from Shougo/ng main to this.
On the virtual GPU: 5.25 to 0.95 ms with OpenGL and 5.34 to 0.90 ms
with OpenGL ES, as on macOS.

## New characters

Scrolling a file of distinct kanji (100 new characters a line) froze ng:
44 s of CPU for 300 lines, and no frame between the first one and the
end. Each character missing from 'guifont' was looked up with
`FcFontSort`, which scores and sorts every installed font: 2.5 ms per
character with the 2,800 fonts of the Mac above. A screen of Japanese
text has hundreds of characters ng has not drawn yet.

ng now sorts once per style and walks that list for each new character
(package font, `ng_fallbacks_t`; on macOS in an order of its own, see
docs/macos.md, "Fallback fonts"):

| | before | after |
| --- | --- | --- |
| `BenchmarkNewCJKGlyphs` (lookup and rasterizing) | 2.52 ms | 19.7 µs |
| First screen of 6,700 distinct kanji, 216x70 | 14.3 s | 0.45 s |
| Scrolling those 300 lines at 120 Hz, CPU | 44 s | 2.1 s user |

While scrolling, ng now waits for Neovim 61% of the time; uploading the
new glyphs to the atlas (one `SDL_UpdateTexture` each) and rasterizing
them take most of the rest.
