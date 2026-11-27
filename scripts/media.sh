#!/bin/sh
# Makes the README's pictures and the launch's media from a copy of the
# repository, drawn by the app itself (internal/app/media_test.go):
#
#	docs/images/    history.png, history-dark.png, staging.png,
#	                settings.png and demo.gif, which the README shows
#	build/media/    social.png, the repository's social preview, and
#	                demo.mp4, the demo for X and the like; the media test
#	                leaves the frames of the demo there too
#
# It needs ffmpeg.
set -eu
cd "$(dirname "$0")/.."
command -v ffmpeg >/dev/null || { echo "media.sh needs ffmpeg" >&2; exit 1; }
out=build/media
# What it makes, and nothing else there, as the launch's drafts.
rm -rf "$out/frames"
LAZIERGIT_MEDIA="$out" go test -count=1 -run '^TestMedia$' ./internal/app

# The demo: 1080p for X, and a GIF the README plays, smaller.
(
	cd "$out/frames"
	ffmpeg -loglevel error -y -f concat -safe 0 -i list.txt \
		-vf "scale=1920:1080:flags=lanczos,fps=30,format=yuv420p" \
		-c:v libx264 -crf 18 -preset slow -movflags +faststart ../demo.mp4
	ffmpeg -loglevel error -y -f concat -safe 0 -i list.txt \
		-vf "fps=12,scale=1100:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
		../demo.gif
)

mkdir -p docs/images
for f in history.png history-dark.png staging.png settings.png demo.gif; do
	cp "$out/$f" docs/images/
done
ls -lh docs/images "$out/demo.mp4" "$out/social.png"
