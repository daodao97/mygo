#!/bin/sh
# Makes the samples of TestFormatMatrix with ffmpeg (libx264, libx265,
# libvpx-vp9, libsvtav1, libopus): each 1.2 s of 64×48 at 10 fps, 0.4 s
# red, then green, then blue, a keyframe at each, with silent sound.
set -eu
export SVT_LOG=1 # SVT-AV1's errors alone
cd "$(dirname "$0")"
rm -rf ./*.mp4 ./*.m4v ./*.3gp ./*.mov ./*.mkv ./*.webm ./*.ts ./*.m2ts ./*.mpg ./*.avi hls_fmp4 hls_ts

colors() { # $1: size, $2: rate
	echo "color=c=red:s=$1:r=$2:d=0.4[r];color=c=lime:s=$1:r=$2:d=0.4[g];color=c=blue:s=$1:r=$2:d=0.4[b];[r][g][b]concat=n=3:v=1:a=0,format=yuv420p[v]"
}
# $1: output, then ffmpeg's output options
make() {
	out=$1
	shift
	ffmpeg -v error -y -f lavfi -i anullsrc=r=48000:cl=mono -filter_complex "$(colors 64x48 10)" \
		-map '[v]' -map 0:a -t 1.2 "$@" "$out"
}
h264='-c:v libx264 -preset veryslow -x264-params bframes=0:keyint=4:min-keyint=4:scenecut=0:log-level=error'
aac='-c:a aac -b:a 24k'
opus='-c:a libopus -b:a 16k'

make mp4_h264_aac.mp4 $h264 $aac
make m4v_h264_aac.m4v $h264 $aac -f mp4 -brand 'M4V '
make 3gp_h264_aac.3gp $h264 $aac
make fmp4_h264_aac.mp4 $h264 $aac -movflags +frag_keyframe+empty_moov+default_base_moof
make mov_h264_aac.mov $h264 $aac
make mkv_h264_aac.mkv $h264 $aac
make ts_h264_aac.ts $h264 $aac
make m2ts_h264_aac.m2ts $h264 $aac -f mpegts -mpegts_m2ts_mode 1
hevc='-c:v libx265 -tag:v hvc1 -x265-params keyint=4:min-keyint=4:bframes=0:scenecut=0:log-level=error'
make mp4_hevc_aac.mp4 $hevc $aac
make mkv_hevc_aac.mkv $hevc $aac
make ts_hevc_aac.ts $hevc $aac
vp9='-c:v libvpx-vp9 -g 4 -keyint_min 4 -b:v 0 -crf 40 -deadline good'
make webm_vp9_opus.webm $vp9 $opus
make mp4_vp9_opus.mp4 $vp9 $opus
# SVT-AV1 takes no picture under 64 high; an AV1 MP4 is fragmented, as
# docs/ui/video.md asks of one that seeks (no sdtp box).
av1() {
	out=$1
	shift
	ffmpeg -v error -y -f lavfi -i anullsrc=r=48000:cl=mono -filter_complex "$(colors 64x64 10)" \
		-map '[v]' -map 0:a -t 1.2 -c:v libsvtav1 -g 4 -svtav1-params keyint=4:scd=0 "$@" "$out"
}
av1 fmp4_av1_aac.mp4 $aac -movflags +frag_keyframe+empty_moov+default_base_moof
av1 webm_av1_opus.webm $opus
# MPEG-2 takes no 10 fps: 25, a keyframe every 10 frames.
ffmpeg -v error -y -f lavfi -i anullsrc=r=48000:cl=mono -filter_complex "$(colors 64x48 25)" \
	-map '[v]' -map 0:a -t 1.2 -c:v mpeg2video -g 10 -bf 0 -q:v 2 -c:a mp2 -b:a 32k -f vob mpg_mpeg2_mp2.mpg
# With its sound, which GStreamer's AVI demuxer reading it over HTTP
# needs: given its last frames in the buffer its headers end in, and no
# buffer after, it drops them, and shows no picture.
make avi_mjpeg_pcm.avi -c:v mjpeg -q:v 3 -c:a pcm_u8 -ar 8000
mkdir hls_fmp4 hls_ts
make hls_fmp4/index.m3u8 $h264 $aac -f hls -hls_time 0.4 -hls_playlist_type vod -hls_segment_type fmp4
make hls_ts/index.m3u8 $h264 $aac -f hls -hls_time 0.4 -hls_playlist_type vod
# ffmpeg rounds the target duration down, to 0, which AVFoundation
# refuses (it is the longest segment's, rounded to the nearest second, and
# at least 1).
for p in hls_fmp4/index.m3u8 hls_ts/index.m3u8; do
	sed 's/^#EXT-X-TARGETDURATION:0$/#EXT-X-TARGETDURATION:1/' "$p" > "$p.new"
	mv "$p.new" "$p"
done
