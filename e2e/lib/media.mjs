// silentMp3 builds a playable MP3 without an encoder: MPEG-1 Layer III
// frames at 128 kbps, 44.1 kHz, mono, whose side info and main data are all
// zero, which decodes as silence. Each frame is 417 bytes (144 * 128000 /
// 44100, no padding) and 1152 samples long, so 100 frames last about 2.6 s.
export function silentMp3(frames = 100) {
  const frame = Buffer.alloc(417);
  frame.set([0xff, 0xfb, 0x90, 0xc4]);
  return Buffer.concat(Array.from({ length: frames }, () => frame));
}
