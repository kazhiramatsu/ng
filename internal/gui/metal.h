#ifndef NG_METAL_H
#define NG_METAL_H

// ng's own Metal renderer for macOS (metal_darwin.m). SDL still makes the
// window and delivers the events.

#include <stdbool.h>
#include <stdint.h>
#include <SDL3/SDL.h>

typedef struct ng_metal ng_metal;

// One rectangle of a frame: a filled rectangle, or a glyph whose coverage
// is the w x h area at (u, v) of the atlas. Positions are in pixels. The
// shader reads this layout; keep the two in step.
typedef struct {
	int16_t x, y;
	uint16_t w, h;
	uint16_t u, v;
	uint8_t r, g, b;
	uint8_t glyph; // 1: a glyph, 0: a filled rectangle
} ng_quad;

// One image of a frame: the source rectangle of a texture, in its pixels,
// scaled into the destination rectangle.
typedef struct {
	void *tex;
	float sx, sy, sw, sh;
	float dx, dy, dw, dh;
} ng_image_draw;

enum {
	NG_METAL_TRANSACTION = 1, // present in step with the window's frame
};

SDL_Window *ng_metal_create_window(const char *title, int w, int h);
// ng_metal_new draws into w; with w NULL it draws into a texture that
// ng_metal_read returns. NULL: Metal is not available.
ng_metal *ng_metal_new(SDL_Window *w, int atlas_size);
void ng_metal_free(ng_metal *m);
const char *ng_metal_device(ng_metal *m);
bool ng_metal_resize_atlas(ng_metal *m, int size);
void ng_metal_upload(ng_metal *m, int x, int y, int w, int h, const void *pix, int stride);
ng_quad *ng_metal_quads(ng_metal *m, int n);
bool ng_metal_frame(ng_metal *m, int w, int h, uint32_t clear_rgb, int grid, int total,
                    const ng_image_draw *images, int nimages, int flags);
bool ng_metal_read(ng_metal *m, void *bgra, int w, int h);
void *ng_metal_texture(ng_metal *m, int w, int h, const void *rgba);
void ng_metal_release_texture(void *tex);
bool ng_metal_in_live_resize(ng_metal *m);
void ng_metal_watch_live_resize(ng_metal *m);

#endif
