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
done.

## Moving rows by renderer

A scroll repainted every row of its region. Rows are painted
independently, so now the painted rows move with their cells
(`screen.TakeChanges`, `paint.Scroll`). The GUI copies them through a
second texture, since SDL cannot draw a texture onto itself.

Scrolling a line every 8 ms, 216x70 (1944x1260 px, 800 flushes), with
rows moved or repainted. Median flush to present (it includes the 4 ms
wait for quiet), and ng's CPU time (user and system) or the frames
presented:

| Renderer | moved | repainted |
| --- | --- | --- |
| OpenGL on a virtual GPU | 4.7 ms, 0.58 s | 5.2 ms, 1.00 s |
| SDL software | 8.9 ms, 370 frames | 10.3 ms, 280 frames |
| OpenGL on llvmpipe | 16 ms, 200 frames | 10 ms, 290 frames |
| Vulkan on lavapipe | 98 ms, 35 frames | 19 ms, 170 frames |

The machine is Ubuntu 26.04 in VirtualBox 7.2 on an Apple M5 Mac. The
virtual GPU is VirtualBox's 3D acceleration, on Xorg: Mesa's SVGA3D
driver hands OpenGL to the host's GPU, so ng's CPU time leaves that work
out. The other rows run on Xvfb, where OpenGL and Vulkan are Mesa's CPU
implementations, as in virtual machines without 3D acceleration.

On a GPU, moving saves most of the painting. With SDL's software
renderer it is a little faster. On llvmpipe and lavapipe the two
full-window copies cost more than painting the rows, and ng falls behind
Neovim. So ng asks the renderer for its device (`ng_renderer_device`:
`GL_RENDERER` or the Vulkan device name) and repaints when it is
llvmpipe, softpipe, SwiftShader or a software rasterizer (`onCPU` in
package gui; the trace says which).
