// ng's own Metal renderer for macOS. A frame is one render pass into the
// window's drawable: every rectangle and glyph of the grid as one
// instanced draw call, the images, then the cursor and the preedit text
// as a second one. Nothing is kept on the GPU between frames but the
// glyph atlas and the images (see metal_darwin.go).
#import <AppKit/AppKit.h>
#import <Metal/Metal.h>
#import <QuartzCore/QuartzCore.h>
#include <SDL3/SDL_metal.h>
#include <stdatomic.h>
#include "metal.h"
#include "_cgo_export.h"

static NSString *const shaders = @""
"#include <metal_stdlib>\n"
"using namespace metal;\n"
"struct Quad { short x, y; ushort w, h; ushort u, v; uchar r, g, b, glyph; };\n"
"struct View { float2 size; float2 atlas; };\n"
"struct QuadOut {\n"
"	float4 pos [[position]];\n"
"	float2 uv;\n"
"	float4 color [[flat]];\n" // a: 1 for a glyph
"};\n"
"vertex QuadOut quad_vertex(uint vid [[vertex_id]], uint iid [[instance_id]],\n"
"		const device Quad *quads [[buffer(0)]], constant View &view [[buffer(1)]]) {\n"
"	Quad q = quads[iid];\n"
"	float2 corner = float2(vid & 1, vid >> 1);\n"
"	float2 size = float2(q.w, q.h);\n"
"	float2 p = float2(q.x, q.y) + corner * size;\n"
"	QuadOut o;\n"
"	o.pos = float4(p.x * 2 / view.size.x - 1, 1 - p.y * 2 / view.size.y, 0, 1);\n"
"	o.uv = (float2(q.u, q.v) + corner * size) / view.atlas;\n"
"	o.color = float4(float3(q.r, q.g, q.b) / 255, q.glyph);\n"
"	return o;\n"
"}\n"
"fragment float4 quad_fragment(QuadOut in [[stage_in]], texture2d<float> atlas [[texture(0)]]) {\n"
"	constexpr sampler s(coord::normalized, filter::nearest);\n"
"	float a = in.color.a > 0.5 ? atlas.sample(s, in.uv).r : 1.0;\n"
"	return float4(in.color.rgb, a);\n"
"}\n"
"struct Image { float4 dst; float4 src; float2 view; };\n"
"struct ImageOut { float4 pos [[position]]; float2 uv; };\n"
"vertex ImageOut image_vertex(uint vid [[vertex_id]], constant Image &im [[buffer(0)]]) {\n"
"	float2 corner = float2(vid & 1, vid >> 1);\n"
"	float2 p = im.dst.xy + corner * im.dst.zw;\n"
"	ImageOut o;\n"
"	o.pos = float4(p.x * 2 / im.view.x - 1, 1 - p.y * 2 / im.view.y, 0, 1);\n"
"	o.uv = im.src.xy + corner * im.src.zw;\n"
"	return o;\n"
"}\n"
"fragment float4 image_fragment(ImageOut in [[stage_in]], texture2d<float> tex [[texture(0)]]) {\n"
"	constexpr sampler s(coord::normalized, filter::linear, address::clamp_to_edge);\n"
"	return tex.sample(s, in.uv);\n"
"}\n";

// A buffer of quads. The GPU reads it after ng_metal_frame returns, so a
// frame takes one that no earlier frame still uses.
@interface NGQuads : NSObject {
@public
	id<MTLBuffer> buf;
	atomic_int busy;
}
@end
@implementation NGQuads
@end

@interface NGMetal : NSObject {
@public
	id<MTLDevice> device;
	id<MTLCommandQueue> queue;
	id<MTLRenderPipelineState> quadPipe, imagePipe;
	id<MTLTexture> atlas;
	int atlasSize;
	NSMutableArray<NGQuads *> *quads;
	NGQuads *cur; // the buffer of the next frame
	SDL_Window *sdlWindow;
	SDL_MetalView view;
	CAMetalLayer *layer;    // nil when drawing into target
	id<MTLTexture> target;  // the frame, without a window
	MTLRenderPassDescriptor *pass;
	uint32_t background;    // the layer's background color, or ~0: not set
	char name[256];
}
@end
@implementation NGMetal
@end

SDL_Window *ng_metal_create_window(const char *title, int w, int h) {
	return SDL_CreateWindow(title, w, h,
		SDL_WINDOW_RESIZABLE | SDL_WINDOW_HIGH_PIXEL_DENSITY | SDL_WINDOW_HIDDEN | SDL_WINDOW_METAL);
}

static id<MTLRenderPipelineState> pipeline(NGMetal *m, id<MTLLibrary> lib, NSString *vertex, NSString *fragment) {
	MTLRenderPipelineDescriptor *d = [MTLRenderPipelineDescriptor new];
	d.vertexFunction = [lib newFunctionWithName:vertex];
	d.fragmentFunction = [lib newFunctionWithName:fragment];
	MTLRenderPipelineColorAttachmentDescriptor *c = d.colorAttachments[0];
	c.pixelFormat = MTLPixelFormatBGRA8Unorm;
	c.blendingEnabled = YES;
	c.sourceRGBBlendFactor = MTLBlendFactorSourceAlpha;
	c.destinationRGBBlendFactor = MTLBlendFactorOneMinusSourceAlpha;
	c.sourceAlphaBlendFactor = MTLBlendFactorOne;
	c.destinationAlphaBlendFactor = MTLBlendFactorOneMinusSourceAlpha;
	return [m->device newRenderPipelineStateWithDescriptor:d error:NULL];
}

static id<MTLTexture> new_atlas(NGMetal *m, int size) {
	MTLTextureDescriptor *td = [MTLTextureDescriptor texture2DDescriptorWithPixelFormat:MTLPixelFormatR8Unorm
		width:size height:size mipmapped:NO];
	return [m->device newTextureWithDescriptor:td];
}

ng_metal *ng_metal_new(SDL_Window *w, int atlas_size) {
	@autoreleasepool {
		NGMetal *m = [NGMetal new];
		m->device = MTLCreateSystemDefaultDevice();
		if (!m->device)
			return NULL;
		m->queue = [m->device newCommandQueue];
		NSError *err = nil;
		id<MTLLibrary> lib = [m->device newLibraryWithSource:shaders options:nil error:&err];
		if (!lib) {
			SDL_Log("ng: Metal shaders: %s", err.localizedDescription.UTF8String);
			return NULL;
		}
		m->quadPipe = pipeline(m, lib, @"quad_vertex", @"quad_fragment");
		m->imagePipe = pipeline(m, lib, @"image_vertex", @"image_fragment");
		if (!m->quadPipe || !m->imagePipe)
			return NULL;
		m->atlas = new_atlas(m, atlas_size);
		m->atlasSize = atlas_size;
		m->quads = [NSMutableArray new];
		m->pass = [MTLRenderPassDescriptor new];
		m->pass.colorAttachments[0].loadAction = MTLLoadActionClear;
		m->pass.colorAttachments[0].storeAction = MTLStoreActionStore;
		m->background = ~0u;
		SDL_strlcpy(m->name, m->device.name.UTF8String, sizeof m->name);
		if (w) {
			m->sdlWindow = w;
			m->view = SDL_Metal_CreateView(w);
			if (!m->view)
				return NULL;
			CAMetalLayer *layer = (__bridge CAMetalLayer *)SDL_Metal_GetLayer(m->view);
			layer.device = m->device;
			layer.pixelFormat = MTLPixelFormatBGRA8Unorm;
			layer.framebufferOnly = YES;
			layer.opaque = YES;
			// Colors are sRGB, as with SDL's renderer.
			CGColorSpaceRef srgb = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
			layer.colorspace = srgb;
			CGColorSpaceRelease(srgb);
			// Two drawables, not three, and no wait for the display's
			// refresh: the window server takes a frame at once
			// (docs/macos.md, "After the present").
			layer.maximumDrawableCount = 2;
			layer.displaySyncEnabled = NO;
			// A frame smaller or larger than the window (a resize ng
			// has not drawn yet) stays at the top left in its own size,
			// rather than stretched to the window.
			layer.contentsGravity = kCAGravityTopLeft;
			m->layer = layer;
		}
		return (ng_metal *)CFBridgingRetain(m);
	}
}

void ng_metal_free(ng_metal *p) {
	NGMetal *m = (NGMetal *)CFBridgingRelease(p);
	if (m->view)
		SDL_Metal_DestroyView(m->view);
}

const char *ng_metal_device(ng_metal *p) { return ((__bridge NGMetal *)p)->name; }

// ng_metal_resize_atlas replaces the atlas with an empty one of another
// size.
bool ng_metal_resize_atlas(ng_metal *p, int size) {
	NGMetal *m = (__bridge NGMetal *)p;
	id<MTLTexture> atlas = new_atlas(m, size);
	if (!atlas)
		return false;
	m->atlas = atlas;
	m->atlasSize = size;
	return true;
}

// ng_metal_upload puts a glyph's coverage into the atlas.
void ng_metal_upload(ng_metal *p, int x, int y, int w, int h, const void *pix, int stride) {
	NGMetal *m = (__bridge NGMetal *)p;
	[m->atlas replaceRegion:MTLRegionMake2D(x, y, w, h) mipmapLevel:0 withBytes:pix bytesPerRow:stride];
}

// ng_metal_quads returns room for the n quads of the next frame.
ng_quad *ng_metal_quads(ng_metal *p, int n) {
	NGMetal *m = (__bridge NGMetal *)p;
	NSUInteger need = (NSUInteger)(n > 0 ? n : 1) * sizeof(ng_quad);
	NGQuads *q = nil;
	for (NGQuads *c in m->quads) {
		if (!atomic_load(&c->busy)) {
			q = c;
			break;
		}
	}
	if (!q) {
		q = [NGQuads new];
		[m->quads addObject:q];
	}
	if (q->buf.length < need) {
		NSUInteger size = 64 * 1024;
		while (size < need)
			size *= 2;
		q->buf = [m->device newBufferWithLength:size options:MTLResourceStorageModeShared];
	}
	m->cur = q;
	return (ng_quad *)q->buf.contents;
}

static void set_background(NGMetal *m, uint32_t rgb) {
	if (m->background == rgb)
		return;
	m->background = rgb;
	CGColorRef c = CGColorCreateSRGB(((rgb >> 16) & 0xff) / 255.0, ((rgb >> 8) & 0xff) / 255.0, (rgb & 0xff) / 255.0, 1);
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	m->layer.backgroundColor = c;
	[CATransaction commit];
	CGColorRelease(c);
}

// ng_metal_frame draws a frame of w x h pixels from the quads written to
// ng_metal_quads: the first `grid` of them, the images over those, then
// the quads up to `total`. With a window it presents the frame; with
// NG_METAL_TRANSACTION as part of the current Core Animation transaction,
// so that it appears together with a new window frame.
bool ng_metal_frame(ng_metal *p, int w, int h, uint32_t clear_rgb, int grid, int total,
                    const ng_image_draw *images, int nimages, int flags) {
	NGMetal *m = (__bridge NGMetal *)p;
	@autoreleasepool {
		NGQuads *quads = m->cur;
		if (!quads || w <= 0 || h <= 0)
			return false;
		id<CAMetalDrawable> drawable = nil;
		id<MTLTexture> target;
		bool transaction = (flags & NG_METAL_TRANSACTION) != 0;
		if (m->layer) {
			CGSize size = CGSizeMake(w, h);
			if (!CGSizeEqualToSize(m->layer.drawableSize, size))
				m->layer.drawableSize = size;
			if (m->layer.presentsWithTransaction != transaction) {
				m->layer.presentsWithTransaction = transaction;
				// With two drawables, a frame in a transaction waited
				// 14 ms for a drawable, and a resize went in steps of
				// 16.7 ms, not 8.3 (docs/macos.md, "Metal renderer").
				m->layer.maximumDrawableCount = transaction ? 3 : 2;
			}
			// What shows where a frame does not cover the window.
			set_background(m, clear_rgb);
			drawable = [m->layer nextDrawable];
			if (!drawable)
				return false;
			target = drawable.texture;
		} else {
			if (!m->target || m->target.width != (NSUInteger)w || m->target.height != (NSUInteger)h) {
				MTLTextureDescriptor *td = [MTLTextureDescriptor texture2DDescriptorWithPixelFormat:MTLPixelFormatBGRA8Unorm
					width:w height:h mipmapped:NO];
				td.usage = MTLTextureUsageRenderTarget;
				m->target = [m->device newTextureWithDescriptor:td];
			}
			target = m->target;
		}

		MTLRenderPassDescriptor *pass = m->pass;
		pass.colorAttachments[0].texture = target;
		pass.colorAttachments[0].clearColor = MTLClearColorMake(((clear_rgb >> 16) & 0xff) / 255.0,
			((clear_rgb >> 8) & 0xff) / 255.0, (clear_rgb & 0xff) / 255.0, 1);
		id<MTLCommandBuffer> cb = [m->queue commandBuffer];
		id<MTLRenderCommandEncoder> enc = [cb renderCommandEncoderWithDescriptor:pass];
		struct { float w, h, aw, ah; } view = { w, h, m->atlasSize, m->atlasSize };
		[enc setRenderPipelineState:m->quadPipe];
		[enc setVertexBuffer:quads->buf offset:0 atIndex:0];
		[enc setVertexBytes:&view length:sizeof view atIndex:1];
		[enc setFragmentTexture:m->atlas atIndex:0];
		if (grid > 0)
			[enc drawPrimitives:MTLPrimitiveTypeTriangleStrip vertexStart:0 vertexCount:4 instanceCount:grid];
		if (nimages > 0) {
			[enc setRenderPipelineState:m->imagePipe];
			for (int i = 0; i < nimages; i++) {
				const ng_image_draw *d = &images[i];
				id<MTLTexture> tex = (__bridge id<MTLTexture>)d->tex;
				float tw = tex.width, th = tex.height;
				struct { float dst[4], src[4], view[2], pad[2]; } im = {
					{ d->dx, d->dy, d->dw, d->dh },
					{ d->sx / tw, d->sy / th, d->sw / tw, d->sh / th },
					{ w, h },
				};
				[enc setVertexBytes:&im length:sizeof im atIndex:0];
				[enc setFragmentTexture:tex atIndex:0];
				[enc drawPrimitives:MTLPrimitiveTypeTriangleStrip vertexStart:0 vertexCount:4];
			}
			[enc setRenderPipelineState:m->quadPipe];
			[enc setVertexBuffer:quads->buf offset:0 atIndex:0];
			[enc setFragmentTexture:m->atlas atIndex:0];
		}
		if (total > grid)
			[enc drawPrimitives:MTLPrimitiveTypeTriangleStrip vertexStart:0 vertexCount:4
				instanceCount:total - grid baseInstance:grid];
		[enc endEncoding];
		pass.colorAttachments[0].texture = nil;

		atomic_store(&quads->busy, 1);
		[cb addCompletedHandler:^(id<MTLCommandBuffer> b) {
			atomic_store(&quads->busy, 0);
		}];
		m->cur = nil;
		if (!drawable) {
			[cb commit];
			[cb waitUntilCompleted];
		} else if (transaction) {
			[cb commit];
			[cb waitUntilScheduled];
			[drawable present];
		} else {
			[cb presentDrawable:drawable];
			[cb commit];
		}
		return true;
	}
}

// ng_metal_read copies the last frame drawn without a window, as BGRA.
bool ng_metal_read(ng_metal *p, void *bgra, int w, int h) {
	NGMetal *m = (__bridge NGMetal *)p;
	if (!m->target || m->target.width != (NSUInteger)w || m->target.height != (NSUInteger)h)
		return false;
	@autoreleasepool {
		if (m->target.storageMode == MTLStorageModeManaged) {
			id<MTLCommandBuffer> cb = [m->queue commandBuffer];
			id<MTLBlitCommandEncoder> blit = [cb blitCommandEncoder];
			[blit synchronizeResource:m->target];
			[blit endEncoding];
			[cb commit];
			[cb waitUntilCompleted];
		}
		[m->target getBytes:bgra bytesPerRow:w * 4 fromRegion:MTLRegionMake2D(0, 0, w, h) mipmapLevel:0];
	}
	return true;
}

// ng_metal_texture makes a texture from non-premultiplied RGBA pixels, for
// an image placed with vim.ui.img. Free it with ng_metal_release_texture.
void *ng_metal_texture(ng_metal *p, int w, int h, const void *rgba) {
	NGMetal *m = (__bridge NGMetal *)p;
	MTLTextureDescriptor *td = [MTLTextureDescriptor texture2DDescriptorWithPixelFormat:MTLPixelFormatRGBA8Unorm
		width:w height:h mipmapped:NO];
	id<MTLTexture> tex = [m->device newTextureWithDescriptor:td];
	if (!tex)
		return NULL;
	[tex replaceRegion:MTLRegionMake2D(0, 0, w, h) mipmapLevel:0 withBytes:rgba bytesPerRow:w * 4];
	return (void *)CFBridgingRetain(tex);
}

void ng_metal_release_texture(void *tex) {
	if (tex)
		CFRelease(tex);
}

static NSWindow *cocoa_window(NGMetal *m) {
	return (__bridge NSWindow *)SDL_GetPointerProperty(SDL_GetWindowProperties(m->sdlWindow),
		SDL_PROP_WINDOW_COCOA_WINDOW_POINTER, NULL);
}

// ng_metal_in_live_resize reports whether macOS is resizing the window in
// its own event loop: the user drags its edge, or the frame is animated.
bool ng_metal_in_live_resize(ng_metal *p) {
	NGMetal *m = (__bridge NGMetal *)p;
	return m->sdlWindow && cocoa_window(m).inLiveResize;
}

// During a live resize SDL_WaitEvent does not return until the resize
// ends, so ng's loop cannot draw. SDL still tells event watchers of each
// new size, from inside AppKit's windowDidResize, and sends them
// SDL_EVENT_WINDOW_EXPOSED with data1 = 1 from a timer, 60 times a second,
// while the user drags. Both draw a frame (ngLiveResize in
// liveresize_darwin.go).
static bool SDLCALL live_resize_watch(void *userdata, SDL_Event *e) {
	bool sized = e->type == SDL_EVENT_WINDOW_PIXEL_SIZE_CHANGED;
	if (!sized && !(e->type == SDL_EVENT_WINDOW_EXPOSED && e->window.data1 == 1))
		return true;
	if ([NSThread isMainThread] && ng_metal_in_live_resize((ng_metal *)userdata))
		ngLiveResize(sized);
	return true;
}

void ng_metal_watch_live_resize(ng_metal *m) { SDL_AddEventWatch(live_resize_watch, m); }
