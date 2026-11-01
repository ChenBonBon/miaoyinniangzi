#!/usr/bin/env python3
"""Opt-in, dependency-free libmpv C API probe; never accesses real source keys.

Example: python3 tools/probe_libmpv.py --library /absolute/path/libmpv.2.dylib
The supplied library must be trusted and its dependencies discoverable. This is
research tooling, not an application backend. Generated audio stays in a private
TemporaryDirectory and is deleted on exit. No audio is sent to speakers.
"""
import argparse
import array
import ctypes as C
import http.server
import json
import math
import os
from pathlib import Path
import struct
import subprocess
import sys
import tempfile
import threading
import time
import wave

RATE = 44100
SECONDS = 40


class Event(C.Structure):
    _fields_ = [("id", C.c_int), ("error", C.c_int),
                ("userdata", C.c_uint64), ("data", C.c_void_p)]


class EndFile(C.Structure):
    _fields_ = [("reason", C.c_int), ("error", C.c_int)]


class Library:
    def __init__(self, path):
        self.directory = os.add_dll_directory(str(path.parent)) if os.name == "nt" else None
        self.lib = C.CDLL(str(path))
        signatures = {
            "mpv_create": (C.c_void_p, []),
            "mpv_initialize": (C.c_int, [C.c_void_p]),
            "mpv_set_option_string": (C.c_int, [C.c_void_p, C.c_char_p, C.c_char_p]),
            "mpv_set_property_string": (C.c_int, [C.c_void_p, C.c_char_p, C.c_char_p]),
            "mpv_get_property_string": (C.c_void_p, [C.c_void_p, C.c_char_p]),
            "mpv_command": (C.c_int, [C.c_void_p, C.POINTER(C.c_char_p)]),
            "mpv_wait_event": (C.POINTER(Event), [C.c_void_p, C.c_double]),
            "mpv_error_string": (C.c_char_p, [C.c_int]),
            "mpv_free": (None, [C.c_void_p]),
            "mpv_terminate_destroy": (None, [C.c_void_p]),
        }
        for name, (result, args) in signatures.items():
            func = getattr(self.lib, name)
            func.restype, func.argtypes = result, args

    def check(self, result):
        if result < 0:
            raise RuntimeError(self.lib.mpv_error_string(result).decode())


class Player:
    def __init__(self, library, options=None):
        self.api, self.lib = library, library.lib
        self.handle = self.lib.mpv_create()
        if not self.handle:
            raise RuntimeError("mpv_create failed")
        defaults = {
            "config": "no", "load-scripts": "no", "ytdl": "no",
            "terminal": "no", "input-default-bindings": "no",
            "video": "no", "audio-display": "no", "ao": "null",
            "idle": "yes", "pause": "yes", "hr-seek": "yes", "keep-open": "always",
            "hr-seek-demuxer-offset": "0.25",
            "demuxer-max-bytes": "128KiB", "demuxer-max-back-bytes": "0",
            "cache-secs": "0.2", "stream-buffer-size": "32KiB",
        }
        defaults.update(options or {})
        try:
            for key, value in defaults.items():
                library.check(self.lib.mpv_set_option_string(
                    self.handle, key.encode(), str(value).encode()))
            library.check(self.lib.mpv_initialize(self.handle))
        except BaseException:
            self.close()
            raise

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()

    def close(self):
        if self.handle:
            self.lib.mpv_terminate_destroy(self.handle)
            self.handle = None

    def command(self, *args):
        values = (C.c_char_p * (len(args) + 1))()
        for index, arg in enumerate(args):
            values[index] = str(arg).encode()
        self.api.check(self.lib.mpv_command(self.handle, values))

    def set(self, key, value):
        self.api.check(self.lib.mpv_set_property_string(
            self.handle, key.encode(), str(value).encode()))

    def get(self, key):
        data = self.lib.mpv_get_property_string(self.handle, key.encode())
        if not data:
            return None
        try:
            return C.string_at(data).decode()
        finally:
            self.lib.mpv_free(data)

    def event(self, timeout=0.05):
        event = self.lib.mpv_wait_event(self.handle, timeout).contents
        if event.id == 7 and event.data:
            end = C.cast(event.data, C.POINTER(EndFile)).contents
            if end.reason == 4:
                self.api.check(end.error)
                raise RuntimeError("media failed")
        return event.id

    def wait(self, event_id, timeout=20):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if self.event() == event_id:
                return
        raise TimeoutError(f"waiting for event {event_id}")

    def load(self, url):
        self.command("loadfile", url)
        self.wait(8)
        self.wait(21)  # Audio output must finish initial setup before a runtime seek.

    def seek(self, target):
        self.command("seek", target, "absolute+exact")
        self.wait(20)
        self.wait(21)
        position = float(self.get("time-pos"))
        if not math.isfinite(position) or abs(position - target) > 0.035:
            raise AssertionError(f"seek acknowledgement mismatch: {position} vs {target}")
        return position


def make_chirp(path):
    samples = array.array("h", (
        round(12000 * math.sin(2 * math.pi * (220 * (i / RATE) + 25 * (i / RATE) ** 2)))
        for i in range(RATE * SECONDS)
    ))
    if sys.byteorder != "little":
        samples.byteswap()
    with wave.open(str(path), "wb") as out:
        out.setparams((1, 2, RATE, 0, "NONE", "not compressed"))
        out.writeframes(samples.tobytes())


def encode(api, source, target, codec, bitrate=None, lame=None):
    if bitrate and lame:
        subprocess.run([str(lame), "--silent", "--cbr", "-b", str(bitrate),
                        str(source), str(target)], check=True, timeout=60,
                       stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        return
    options = {"o": str(target), "of": "flac" if codec == "flac" else "mp3",
               "oac": codec, "pause": "no", "ao": "lavc", "keep-open": "no"}
    if bitrate:
        options["oacopts"] = f"b={bitrate}k"
    with Player(api, options) as player:
        player.command("loadfile", str(source))
        player.wait(7, 45)
    if not target.exists() or target.stat().st_size < 1000:
        raise AssertionError("encoder produced no audio")


class RangeServer:
    def __init__(self, files):
        self.files, self.requests, self.bytes = files, [], 0
        owner = self

        class Handler(http.server.BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *_):
                pass

            def do_HEAD(self):
                self.respond(False)

            def do_GET(self):
                self.respond(True)

            def respond(self, body):
                file = owner.files.get(self.path)
                if not file:
                    self.send_error(404)
                    return
                size = file.stat().st_size
                start, end = 0, size - 1
                header = self.headers.get("Range")
                if header:
                    try:
                        assert header.startswith("bytes=") and "," not in header
                        left, right = header[6:].split("-", 1)
                        start = int(left) if left else max(0, size - int(right))
                        end = min(size - 1, int(right)) if right and left else size - 1
                        assert 0 <= start <= end < size
                    except (ValueError, AssertionError):
                        self.send_error(416)
                        return
                owner.requests.append((self.path, start, bool(header)))
                self.send_response(206 if header else 200)
                self.send_header("Content-Length", str(end - start + 1))
                self.send_header("Content-Type", "audio/flac" if file.suffix == ".flac" else "audio/mpeg")
                self.send_header("Accept-Ranges", "bytes")
                if header:
                    self.send_header("Content-Range", f"bytes {start}-{end}/{size}")
                self.end_headers()
                if body:
                    try:
                        with file.open("rb") as stream:
                            stream.seek(start)
                            remaining = end - start + 1
                            while remaining:
                                data = stream.read(min(16384, remaining))
                                if not data:
                                    break
                                self.wfile.write(data)
                                owner.bytes += len(data)
                                remaining -= len(data)
                    except (BrokenPipeError, ConnectionResetError):
                        pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.server.daemon_threads = True
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def url(self, name):
        return f"http://127.0.0.1:{self.server.server_port}/{name}"

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


def read_pcm(path):
    raw = path.read_bytes()
    if raw[:4] != b"RIFF" or raw[8:12] != b"WAVE":
        raise AssertionError("PCM output is not RIFF/WAVE")
    offset, fmt, data = 12, None, None
    while offset + 8 <= len(raw):
        tag, size = raw[offset:offset + 4], struct.unpack_from("<I", raw, offset + 4)[0]
        chunk = raw[offset + 8:offset + 8 + size]
        if tag == b"fmt ":
            fmt = struct.unpack_from("<HHIIHH", chunk)
        elif tag == b"data":
            data = chunk
            break
        offset += 8 + size + size % 2
    if not fmt or data is None or fmt[0] not in (1, 65534) or fmt[1] != 1 or fmt[2] != RATE or fmt[5] != 16:
        raise AssertionError("unexpected PCM format")
    samples = array.array("h")
    samples.frombytes(data[:len(data) // 2 * 2])
    if sys.byteorder != "little":
        samples.byteswap()
    return samples


def verify_pcm(api, url, target, path):
    options = {"ao": "pcm", "ao-pcm-file": str(path),
               "audio-format": "s16", "audio-samplerate": RATE,
               "audio-channels": "mono", "start": target, "pause": "no",
               "end": min(SECONDS, target + 0.3), "keep-open": "no"}
    # The PCM file AO is untimed and cannot reliably emulate a paused device.
    # Check decoded start-position content here; runtime seek state is checked
    # separately with the timed null AO. This is not a physical output capture.
    with Player(api, options) as player:
        player.command("loadfile", url)
        player.wait(7)
    samples = read_pcm(path)
    if len(samples) < 300:
        raise AssertionError(f"not enough decoded samples: {len(samples)}")
    # Lossy decoder priming and cut-edge ringing are not stable frequency
    # measurements. Measure the central half, retaining its original time offset.
    trim = len(samples) // 4
    crossings = [i - 1 + (-samples[i - 1] / (samples[i] - samples[i - 1]))
                 for i in range(max(1, trim), len(samples) - trim)
                 if samples[i - 1] <= 0 < samples[i]]
    if len(crossings) < 8:
        raise AssertionError("decoded signal missing")
    # The known source has instantaneous frequency 220 + 50*t Hz. Rising
    # crossings identify the actual decoded time, independently of time-pos.
    first, last = crossings[0], crossings[-1]
    frequency = (len(crossings) - 1) * RATE / (last - first)
    center = (first + last) / (2 * RATE)
    error = (frequency - 220) / 50 - target - center
    if abs(error) > 0.025:
        raise AssertionError(f"decoded signal seek error {error:.6f}s at {target}s")
    return {"target": target, "samples": len(samples), "pcm_error_ms": round(error * 1000, 3)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--library", required=True, type=Path)
    parser.add_argument("--lame", type=Path, help="Optional LAME encoder if libmpv lacks libmp3lame")
    parser.add_argument("--device-smoke", action="store_true", help="Use a real audio device, always muted")
    args = parser.parse_args()
    api = Library(args.library.resolve(strict=True))
    with Player(api) as player:
        version = player.get("mpv-version")
        ffmpeg = player.get("ffmpeg-version")
    audio_output = ("coreaudio" if sys.platform == "darwin" else "wasapi" if os.name == "nt" else "auto") if args.device_smoke else "null"
    report = {"platform": sys.platform, "mpv": version, "ffmpeg": ffmpeg,
              "runtime_audio_output": audio_output, "formats": {}}
    with tempfile.TemporaryDirectory(prefix="miaoyin-libmpv-probe-") as directory:
        root = Path(directory)
        source = root / "chirp.wav"
        make_chirp(source)
        formats = {"flac": ("flac", None), "320k": ("libmp3lame", 320), "128k": ("libmp3lame", 128)}
        files = {}
        for name, (codec, bitrate) in formats.items():
            target = root / (name + (".flac" if name == "flac" else ".mp3"))
            encode(api, source, target, codec, bitrate, args.lame)
            files["/" + name] = target
        server = RangeServer(files)
        try:
            for name in formats:
                url = server.url(name)
                pcm = []
                for target in [20.0, 39.2, 39.96, 2.0]:
                    print(f"probe {name} at {target}s", file=sys.stderr)
                    pcm.append(verify_pcm(api, url, target, root / f"{name}-{target}.wav"))
                with Player(api, {"ao": audio_output, "mute": "yes"}) as player:
                    player.load(url)
                    paused = player.seek(39.96)
                    if player.get("pause") != "yes":
                        raise AssertionError("paused endpoint resumed")
                    player.seek(2)
                    player.set("pause", "no")
                    deadline = time.monotonic() + 2
                    while float(player.get("time-pos") or 0) < 2.12 and time.monotonic() < deadline:
                        player.event()
                    if float(player.get("time-pos") or 0) < 2.12:
                        raise AssertionError("playback clock did not advance")
                    player.seek(20)
                    if player.get("pause") != "no":
                        raise AssertionError("playing seek paused")
                    player.set("volume", "37")
                    if float(player.get("volume")) != 37:
                        raise AssertionError("volume failed")
                    player.set("pause", "yes")
                    player.command("stop")
                ranges = [start for path, start, ranged in server.requests
                          if path == "/" + name and ranged and start > 0]
                if not ranges:
                    raise AssertionError("no nonzero HTTP Range requests observed")
                report["formats"][name] = {
                    "start_position_pcm": pcm, "paused_endpoint": paused,
                    "playing_seek": "passed", "volume_stop": "passed",
                    "nonzero_range_requests": len(ranges),
                }
        finally:
            server.close()
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
