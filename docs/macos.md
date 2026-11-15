# macOS

2026-11-14. macOS 26.5 on Apple silicon (ANSI keyboard, Japanese locale),
SDL 3.4.20, FreeType, fontconfig and Neovim 0.12.6 from Homebrew.

## Building

Only `internal/detach` did not compile: `syscall.Dup3` and `TCGETS` are
Linux's (macOS has `Dup2` and `TIOCGETA`). Once built, SDL_Init failed:
`SDL_HINT_VIDEO_DRIVER` "wayland,x11" names no driver macOS has (its
driver is "cocoa"), so the hint is now set on Linux only. Shougo/ng has
since made both changes itself (5e82bb9), so that macOS forks can follow
it with fewer conflicts.

## The default font

With 'guifont' empty, "monospace" was Hiragino Maru Gothic, a font with
proportional Latin letters: `Hello` came out spread over wide cells.
fontconfig's own aliases for monospace list only fonts found on Linux,
and with fontconfig 2.17.1 under a Japanese locale its language criterion
outranked those (weak) aliases:

```sh
fc-match monospace                    # ヒラギノ丸ゴ ProN W4.ttc (LANG=ja_JP.UTF-8)
LANG=en_US.UTF-8 fc-match monospace   # Andale Mono.ttf
```

fontconfig 2.18 matches generic families on their own (`genericfamily`,
`48-guessfamily.conf`): there "monospace" is Andale Mono under any
locale. That is a monospace font, but not the one of macOS applications,
and "SF Mono" (below) is still not found.

Neovim 0.12 sets 'guifont' on macOS itself, to "SF Mono,Menlo,Monaco,
Courier New,monospace", and ng then warned `font: "SF Mono" not found` at
every start. macOS installs SF Mono as the hidden system font
".SF NS Mono", which CoreText finds as "SF Mono" and fontconfig does not:

```sh
nvim --clean --headless -c 'lua io.stdout:write(vim.o.guifont)' -c q
fc-list | grep SFNSMono               # .SF NS Mono, with Bold and Italic instances
```

On macOS ng now looks for "monospace" as Menlo and "SF Mono" as
".SF NS Mono" (`ng_alias` in package font), after the name itself, so a
font installed under that name still wins. FreeType renders the hidden
font's named instances (Regular, Bold, Italic) as real faces.

Sizes: macOS applications take a point as one unit of window coordinates
(72 dpi), not GTK's 96, so ng does too there. Menlo 11 is then as large
as in MacVim and Terminal.

## Fallback fonts

Japanese text came out in Nanum Gothic, a Korean font that macOS
installs: kanji in their Korean shapes, and Hiragino only for the few
characters Nanum lacks (𠮷, ㍿, half-width kana). Armenian, Devanagari,
Khmer and many emoji came out as LastResort's boxes, although macOS has
fonts for them.

ng drew a character that 'guifont' lacks with the first font fontconfig
sorted for a request naming that character. The order starts with the
fonts of fontconfig's monospace aliases, which list Nanum Gothic and none
of the Japanese fonts macOS has:

```sh
LANG=ja_JP.UTF-8 fc-match -a ':spacing=100:weight=80:slant=0' family | head
# Andale Mono, Courier New, Courier, Nanum Gothic, Menlo, Monaco, ...
```

LastResort (`/System/Library/Fonts/LastResort.otf`) maps every code point
to a box naming its Unicode block, and says it is monospaced: it is 236th
of the 2,800 fonts in that order, ahead of most fonts for other scripts.
With the character in the request, fontconfig also prefers a font that
supports the locale's language, and LastResort supports all of them:

```sh
fc-match -s ':spacing=100:charset=554' family            # .LastResort
fc-match -s ':spacing=100:charset=554:lang=hy' family    # Noto Sans Armenian
```

macOS itself (Core Text, and so Terminal and MacVim) falls back along a
cascade list per language. For Menlo, `CTFontCopyDefaultCascadeListForLanguages`
gives Monaco, Apple Symbols, Courier New, Helvetica, Lucida Grande, then
Hiragino Sans, PingFang SC and TC for Japanese, English or German; Apple
SD Gothic Neo first for Korean, PingFang first for Chinese. LastResort
only shows when no font has the character.

ng now sorts once per style without the character (as Pango does; see
docs/latency.md, "New characters", for the speed) and takes the first
font that has it, with two changes to fontconfig's order: on macOS the
Chinese, Japanese and Korean fonts of Core Text's list come first, picked
by fontconfig's language (`ng_fallback_first` in package font), and
LastResort comes last. Japanese is now Hiragino Sans (W4, and W7 for
bold), Korean Apple SD Gothic Neo, Armenian Noto Sans Armenian,
Devanagari Kohinoor Devanagari and emoji Apple Color Emoji (in one
colour, like all text), the fonts Core Text picks. `TestMacFallbacks`
checks this under Japanese, English and Korean.

With the 2,800 fonts of this Mac, a new character took 2.5 ms before
sorting once per style and 19.7 µs after (`BenchmarkNewCJKGlyphs`). The
first screen of 6,700 distinct kanji (216x70) took 14.3 s before and
0.45 s after, and scrolling those 300 lines at 120 Hz 44 s of CPU before
and 2.1 s after.

## The window opened behind the terminal

Started from iTerm2, ng's window appeared but iTerm2 stayed the active
application, and typing went to the terminal. Since macOS 14, SDL no
longer activates the application at startup
(`SDL_HINT_MAC_BACKGROUND_APP` defaults to on there, see
`src/video/cocoa/SDL_cocoaevents.m`), and another process cannot do it:
`NSRunningApplication.activate` returned true and changed nothing
(cooperative activation). ng calls SDL_RaiseWindow when it first shows
its window, on macOS only: on GNOME Wayland a raise nobody asked for
becomes an "is ready" notification.

## Keys typed right after Esc were sent twice

`<Esc>0r` and Option+A, arriving together after a stall, gave
`råbcå€` instead of `åbcå€`. Cocoa interprets every pending key in one
SDL_PumpEvents, while text input is still on, so each key after `<Esc>`
made both a key event and a text input event. ng turns text input off at
`<Esc>` and makes the text from the key event (`input.DirectText`), and
then sent the text input event as well. Stopping ng while the keys are
posted reproduces it every time (`keys` is the helper at the end):

```sh
kill -STOP $PID; keys $PID 53 29 15 0+alt; kill -CONT $PID
```

ng now drops text input events that arrive while its text input is off.
This can happen on Linux too, whenever ng is busy while keys are typed
(`<Esc>dd` deleting two lines).

## Option and Cmd

- SDL's key event for Option+L is `l` with Alt. The character the layout
  gives it ("¬" on US, "@" on German) is in SDL's keymap under
  `SDL_KMOD_ALT` on macOS. Taking Option as Alt made such characters
  impossible to type, including "\" (Option+¥) on Japanese keyboards. ng
  takes Option as AltGr for printable keys (`input.MacOption`).
- With text input on, Option+E (an accent) never reaches ng as a key
  event: the input method takes it. `g:ng_macmeta` therefore also sets
  `SDL_HINT_MAC_OPTION_AS_ALT`.
- SDL's application menu acts on Cmd+Q, Cmd+W, Cmd+H, Cmd+M and
  Ctrl+Cmd+F, but SDL still sends their key events; ng drops them so that
  Neovim does not also get `<D-q>`.
- An unmapped `<D-s>` in Insert mode inserts the text `<D-s>`. That is
  Neovim's doing:

  ```sh
  nvim --clean --headless -c 'call feedkeys("i\<D-s>x\<Esc>", "xt")' \
      -c 'lua io.stdout:write(vim.fn.getline(1))' -c 'qa!'   # <D-s>x
  ```

## Trackpad

SDL makes ten points of trackpad movement one wheel step (`y` is
`scrollingDeltaY * 0.1`, `Cocoa_HandleMouseWheel`). ng sent one wheel
event per SDL event whatever its size, three lines for each small
movement. It now adds the steps up (`input.Wheel`): 20 events of 3 pixels
scroll 18 lines. ng also reversed the values of `SDL_MOUSEWHEEL_FLIPPED`
events, which undid "natural scrolling", the macOS default (and under
Wayland the same setting): the values already follow it.

## Latency

`ng --trace` while typing: median 5.8 ms from key event to present (frame
5.0 ms, mostly the 4 ms quiet wait). A first measurement said 20 to 30
ms: the frame dump below was still in `present`. Presenting the final
flush at once (docs/latency.md) then took the key event to present from
a median of 5.6 ms to 1.0 ms (frame 4.8 to 0.25 ms) with keys posted to
ng.

Scrolling a line every 8 ms, 216x70 (3024x1836 px), with all the changes
of performance together: flush to present 5.26 to 0.20 ms, ng's CPU time
0.93 to 0.57 s (user). With scrolled rows moved (docs/latency.md,
"Moving rows by renderer") or repainted, median flush to present and
ng's CPU time (user and system):

| Renderer | moved | repainted |
| --- | --- | --- |
| Metal | 0.22 ms, 1.02 s | 0.42 ms, 1.14 s |
| SDL software | 4.86 ms, 5.30 s | 4.65 ms, 5.07 s |
| SDL software, 120x40 (1680x1080 px) | 1.74 ms, 2.23 s | 2.08 ms, 2.34 s |

### After the present

2026-11-15, MacBook Pro (M5, 120 Hz ProMotion), ng's window not active.

`--trace` ends at SDL_RenderPresent. A temporary build logged each
drawable's `presentedTime` (`MTLDrawable addPresentedHandler`), typing
`j` over RPC at several rhythms. Present to `presentedTime`, in ms:

| Drawables (`maximumDrawableCount`) | Median | p90 |
| --- | --- | --- |
| 3 (the default) | 20.5 | 23.6 |
| 2 | 3.4 | 7.9 |

With three drawables the times lie on the display's 8.3 ms grid, two
refreshes after the present, and vsync on made no difference. Frames
coming faster than the display (`<C-e>` every 8 ms) blocked in
`nextDrawable` for 8 ms each: flush to present 3.5 ms instead of 0.2 ms,
and 35 ms from flush to `presentedTime`.

With two, `nextDrawable` never waited, and the times are not on the grid:
they say when the window server took the frame, not when it was on the
glass. A capture of ng's own window showed the new pixels in the window
server's output about 5 ms after the present. So the real gain is
probably one refresh (8 ms), not the 17 ms of the table; measuring it
needs a camera. ng now asks for two (`ng_low_latency`).

With ng's window active (one run each, the same rhythms), present to
`presentedTime` was 20.6 ms (p90 24) with three drawables and 4.3 ms
(p90 7) with two. Switching to another application and back twice
during the run with two did not stall ng.

Once in the runs with the window not active, when another window was
brought in front of ng's, one `nextDrawable` waited for its 1 s timeout;
44 moves of ng's window under other windows and back did not repeat it,
with two or three drawables.

## Resizing

2026-11-15, the same MacBook Pro.

While the user drags the window's edge (a live resize), macOS runs its
own event loop: `SDL_WaitEvent` does not return until the mouse button
is released, ng cannot draw, and macOS shows the last frame stretched to
the new size. ng draws the new size when the button is released, but
the window kept showing the stretched frame until the next key.

A temporary build logged each drawable's `presentedTime`. The frames
presented up to 16 ms after a live resize ended never reached the
display; at 27 to 29 ms some did, and from 50 ms on all did. This was
the same with two or three drawables, and with resizes animated by
`setFrame:display:animate:`, which macOS also runs as a live resize. A
capture of ng's own window agreed: its content stopped changing 12 ms
after the resize. So ng presents again 100 ms after the window size
changes (`presentAgainLater`). Resized by hand, the frames 3 to 8 ms
after the release were not shown and those 100 ms after were, in 7 of 7
resizes.

Drawing during the live resize was tried too: SDL sends
`SDL_EVENT_WINDOW_EXPOSED` with data1 = 1 to event watchers 60 times a
second while macOS runs its loop. About half of those frames were shown,
and the window flickered and shook while dragged, because the frames do
not change in step with the window's frame. That would need
`presentsWithTransaction` and a different present in SDL's Metal
renderer, so ng does not draw during a live resize.

## Tests

- `TestSurvivesGUIKill` failed now and then: its socket path under
  `t.TempDir()` in `$TMPDIR` (`/var/folders/...`) was 105 bytes, and a
  Unix socket path must fit in 104 on macOS, so Neovim could not listen.
  Shougo/ng now uses a short directory there (5e82bb9). ng also looks
  for the servers it left behind in `$TMPDIR`, where it puts its sockets
  on macOS, so the test points `$TMPDIR` at that directory too: the
  sockets of an ng in use do not count.
- `go test -race ./internal/detach` failed: `Process.Release` raced with
  `cmd.Wait` in another goroutine. Linux waits through a pidfd, where the
  race detector sees nothing.
- Font tests that ask for the fonts CI installs on Linux skipped on macOS
  (`TestGVimSyntaxAndWideSize` needs Courier 10 Pitch and IPAGothic). They
  now fall back to Menlo and Hiragino Sans (`installed` in
  `font_test.go`).
- `TestMacFamilies` checks the aliases above. The worst case needed a
  Japanese locale, and fontconfig reads the language once per process, so
  `TestMacFamiliesJapanese` runs it again with `FC_LANG=ja` (CI runs in
  English). Without `ng_alias` both fail: "monospace" is Andale Mono, or
  Hiragino Maru Gothic in the Japanese process with fontconfig 2.17, and
  "SF Mono" is not found.

  ```sh
  FC_LANG=ja fc-match monospace   # 2.17.1: ヒラギノ丸ゴ Pro, 2.18.3: Andale Mono
  ```

- `TestIsTerminal` needs a terminal's own end of a pseudo-terminal: on
  macOS the master end of `/dev/ptmx` has no terminal attributes
  (`TIOCGETA` fails there, while Linux answers `TCGETS` on it). The
  helpers in `pty_*_test.go` open the other end with each system's
  ioctls. With Linux's `TCGETS` value in `sys_darwin.go` the test fails.
- The Nerd Font tests pass on macOS with UDEV Gothic NF. To run them
  without installing the font, give fontconfig a configuration that adds
  its directory:

  ```xml
  <fontconfig>
    <include ignore_missing="no">/opt/homebrew/etc/fonts/fonts.conf</include>
    <dir>/path/to/UDEVGothic_NF_v2.2.0</dir>
  </fontconfig>
  ```

  ```sh
  FONTCONFIG_FILE=/path/to/fonts.conf go test -run TestWideSymbol ./internal/font/ ./internal/paint/
  ```

- After `brew upgrade` had replaced fontconfig 2.17.1 with 2.18.3, the
  tests did not link: `ld: library 'fontconfig' not found`, with
  `-L/opt/homebrew/Cellar/fontconfig/2.17.1/lib`. Homebrew's
  `fontconfig.pc` names the versioned Cellar directory, and Go's build
  cache does not include pkg-config's output in its key
  (`buildActionID` in `cmd/go/internal/work/exec.go` hashes only the
  `#cgo` flags), so packages built before the upgrade kept the old path.
  `go clean -cache` fixes it. A test package whose own files changed is
  rebuilt and links; the packages that import it from the cache do not.
- `go test -race` prints `ld: warning: ... malformed LC_DYSYMTAB` with
  the current Xcode linker. It is only a warning.

## Testing without hands

- Key events posted to ng's process (CGEvent `postToPid`; the terminal
  needs the Accessibility permission) reach SDL only while ng is the
  active application: SDL drops keys when no window has keyboard focus.
  Start a new ng, which raises itself, rather than activating one.
- Scroll events posted to a process have no window and are dropped. Post
  them through the session event tap, with ng active and the pointer
  over its window.
- `screencapture -l` failed for ng's window ("could not create image from
  window"); a temporary dump of `SDL_RenderReadPixels` before
  `SDL_RenderPresent` showed the frames instead.
- `TISSelectInputSource` switches to
  `com.apple.inputmethod.Kotoeri.RomajiTyping.Japanese` and back to
  `com.apple.keylayout.ABC`, to test the input method. Select it before
  starting ng: an ng started with ABC selected, then switched to Kotoeri
  while it ran, received the posted keys as plain ASCII (twice), while an
  ng started with Kotoeri selected composed and converted them.

The key helper (`swiftc -O -o keys keys.swift`), used as
`keys PID 5+shift 31 0+alt 53` for `G`, `o`, Option+A and Esc. Arguments
are macOS virtual key codes with modifiers:

```swift
import CoreGraphics
import Foundation

let pid = pid_t(CommandLine.arguments[1])!
let mods: [String: (CGKeyCode, CGEventFlags)] = [
    "shift": (56, .maskShift), "ctrl": (59, .maskControl),
    "alt": (58, .maskAlternate), "cmd": (55, .maskCommand),
]
func post(_ key: CGKeyCode, _ down: Bool, _ flags: CGEventFlags) {
    let e = CGEvent(keyboardEventSource: nil, virtualKey: key, keyDown: down)!
    e.flags = flags
    e.postToPid(pid)
    usleep(15000)
}
for spec in CommandLine.arguments.dropFirst(2) {
    let parts = spec.split(separator: "+").map(String.init)
    var flags: CGEventFlags = []
    for m in parts.dropFirst() { flags.insert(mods[m]!.1); post(mods[m]!.0, true, flags) }
    post(CGKeyCode(parts[0])!, true, flags)
    post(CGKeyCode(parts[0])!, false, flags)
    for m in parts.dropFirst().reversed() { flags.remove(mods[m]!.1); post(mods[m]!.0, false, flags) }
}
```
