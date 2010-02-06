// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package draw provides basic graphics and drawing primitives,
// in the style of the Plan 9 graphics library
// (see http://plan9.bell-labs.com/magic/man2html/2/draw)
// and the X Render extension.
package draw

// BUG(rsc): This is a toy library and not ready for production use.

import "image"

// A Porter-Duff compositing operator.
type Op int

const SoverD Op = 0

// A draw.Image is an image.Image with a Set method to change a single pixel.
type Image interface {
	image.Image
	Set(x, y int, c image.Color)
}

// Draw calls DrawMask with a nil mask and an SoverD op.
func Draw(dst Image, r Rectangle, src image.Image, sp Point) {
	DrawMask(dst, r, src, sp, nil, ZP, SoverD)
}

// DrawMask aligns r.Min in dst with sp in src and mp in mask and then replaces the rectangle r
// in dst with the result of a Porter-Duff composition. For the SoverD operator, the result
// is ``(src in mask) over dst''. If mask is nil, this simplifies to ``src over dst''.
// The implementation is simple and slow.
// TODO(nigeltao): Optimize this.
func DrawMask(dst Image, r Rectangle, src image.Image, sp Point, mask image.Image, mp Point, op Op) {
	dx, dy := src.Width()-sp.X, src.Height()-sp.Y
	if mask != nil {
		if dx > mask.Width()-mp.X {
			dx = mask.Width() - mp.X
		}
		if dy > mask.Height()-mp.Y {
			dy = mask.Height() - mp.Y
		}
	}
	if r.Dx() > dx {
		r.Max.X = r.Min.X + dx
	}
	if r.Dy() > dy {
		r.Max.Y = r.Min.Y + dy
	}

	// TODO(nigeltao): Clip r to dst's bounding box, and handle the case when sp or mp has negative X or Y.
	// TODO(nigeltao): Ensure that r is well formed, i.e. r.Max.X >= r.Min.X and likewise for Y.

	// Fast paths for special cases. If none of them apply, then we fall back to a general but slow implementation.
	if dst0, ok := dst.(*image.RGBA); ok && op == SoverD {
		if mask == nil {
			if src0, ok := src.(image.ColorImage); ok {
				drawFill(dst0, r, src0)
				return
			}
			if src0, ok := src.(*image.RGBA); ok {
				if dst0 == src0 && r.Overlaps(r.Add(sp.Sub(r.Min))) {
					// TODO(nigeltao): Implement a fast path for the overlapping case.
				} else {
					drawCopy(dst0, r, src0, sp)
					return
				}
			}
		}
		// TODO(nigeltao): Implement a fast path for font glyphs (i.e. when mask is an image.Alpha).
	}

	x0, x1, dx := r.Min.X, r.Max.X, 1
	y0, y1, dy := r.Min.Y, r.Max.Y, 1
	if image.Image(dst) == src && r.Overlaps(r.Add(sp.Sub(r.Min))) {
		// Rectangles overlap: process backward?
		if sp.Y < r.Min.Y || sp.Y == r.Min.Y && sp.X < r.Min.X {
			x0, x1, dx = x1-1, x0-1, -1
			y0, y1, dy = y1-1, y0-1, -1
		}
	}

	var out *image.RGBA64Color
	sy := sp.Y + y0 - r.Min.Y
	my := mp.Y + y0 - r.Min.Y
	for y := y0; y != y1; y, sy, my = y+dy, sy+dy, my+dy {
		sx := sp.X + x0 - r.Min.X
		mx := mp.X + x0 - r.Min.X
		for x := x0; x != x1; x, sx, mx = x+dx, sx+dx, mx+dx {
			// TODO(nigeltao): Check that op == SoverD.
			if mask == nil {
				dst.Set(x, y, src.At(sx, sy))
				continue
			}
			_, _, _, ma := mask.At(mx, my).RGBA()
			switch ma {
			case 0:
				continue
			case 0xFFFFFFFF:
				dst.Set(x, y, src.At(sx, sy))
			default:
				dr, dg, db, da := dst.At(x, y).RGBA()
				dr >>= 16
				dg >>= 16
				db >>= 16
				da >>= 16
				sr, sg, sb, sa := src.At(sx, sy).RGBA()
				sr >>= 16
				sg >>= 16
				sb >>= 16
				sa >>= 16
				ma >>= 16
				const M = 1<<16 - 1
				a := sa * ma / M
				dr = (dr*(M-a) + sr*ma) / M
				dg = (dg*(M-a) + sg*ma) / M
				db = (db*(M-a) + sb*ma) / M
				da = (da*(M-a) + sa*ma) / M
				if out == nil {
					out = new(image.RGBA64Color)
				}
				out.R = uint16(dr)
				out.G = uint16(dg)
				out.B = uint16(db)
				out.A = uint16(da)
				dst.Set(x, y, out)
			}
		}
	}
}

func drawFill(dst *image.RGBA, r Rectangle, src image.ColorImage) {
	if r.Dy() < 1 {
		return
	}
	cr, cg, cb, ca := src.RGBA()
	color := image.RGBAColor{uint8(cr >> 24), uint8(cg >> 24), uint8(cb >> 24), uint8(ca >> 24)}
	// The built-in copy function is faster than a straightforward for loop to fill the destination with
	// the color, but copy requires a slice source. We therefore use a for loop to fill the first row, and
	// then use the first row as the slice source for the remaining rows.
	dx0, dx1 := r.Min.X, r.Max.X
	dy0, dy1 := r.Min.Y, r.Max.Y
	firstRow := dst.Pixel[dy0]
	for x := dx0; x < dx1; x++ {
		firstRow[x] = color
	}
	copySrc := firstRow[dx0:dx1]
	for y := dy0 + 1; y < dy1; y++ {
		copy(dst.Pixel[y][dx0:dx1], copySrc)
	}
}

func drawCopy(dst *image.RGBA, r Rectangle, src *image.RGBA, sp Point) {
	dx0, dx1 := r.Min.X, r.Max.X
	dy0, dy1 := r.Min.Y, r.Max.Y
	sx0, sx1 := sp.X, sp.X+dx1-dx0
	for y, sy := dy0, sp.Y; y < dy1; y, sy = y+1, sy+1 {
		copy(dst.Pixel[y][dx0:dx1], src.Pixel[sy][sx0:sx1])
	}
}

// Border aligns r.Min in dst with sp in src and then replaces pixels
// in a w-pixel border around r in dst with the result of the Porter-Duff compositing
// operation ``src over dst.''  If w is positive, the border extends w pixels inside r.
// If w is negative, the border extends w pixels outside r.
func Border(dst Image, r Rectangle, w int, src image.Image, sp Point) {
	i := w
	if i > 0 {
		// inside r
		Draw(dst, Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+i), src, sp)                          // top
		Draw(dst, Rect(r.Min.X, r.Min.Y+i, r.Min.X+i, r.Max.Y-i), src, sp.Add(Pt(0, i)))        // left
		Draw(dst, Rect(r.Max.X-i, r.Min.Y+i, r.Max.X, r.Max.Y-i), src, sp.Add(Pt(r.Dx()-i, i))) // right
		Draw(dst, Rect(r.Min.X, r.Max.Y-i, r.Max.X, r.Max.Y), src, sp.Add(Pt(0, r.Dy()-i)))     // bottom
		return
	}

	// outside r;
	i = -i
	Draw(dst, Rect(r.Min.X-i, r.Min.Y-i, r.Max.X+i, r.Min.Y), src, sp.Add(Pt(-i, -i))) // top
	Draw(dst, Rect(r.Min.X-i, r.Min.Y, r.Min.X, r.Max.Y), src, sp.Add(Pt(-i, 0)))      // left
	Draw(dst, Rect(r.Max.X, r.Min.Y, r.Max.X+i, r.Max.Y), src, sp.Add(Pt(r.Dx(), 0)))  // right
	Draw(dst, Rect(r.Min.X-i, r.Max.Y, r.Max.X+i, r.Max.Y+i), src, sp.Add(Pt(-i, 0)))  // bottom
}
