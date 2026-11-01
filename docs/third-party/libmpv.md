# libmpv integration build

This local development build contains libmpv 0.41.0 and its transitive dynamic
libraries, extracted from the official IINA v1.5.0 universal macOS distribution,
thinned to arm64 and relocated with install_name_tool. No library source code
has been modified by this integration.

Upstream artifact SHA-256:
`8fad50479b10e09645053457cec46bd53d054ced2242cfe68f74586735a20dc7`

- Artifact: https://github.com/iina/iina/releases/tag/v1.5.0
- mpv source and licensing: https://github.com/mpv-player/mpv/tree/v0.41.0
- mpv license details: https://github.com/mpv-player/mpv/blob/v0.41.0/Copyright
- IINA dependency build recipes: https://github.com/iina/iina-deps-build
- FFmpeg: https://ffmpeg.org/legal.html
- Go FFI bridge (purego 0.11.2, Apache-2.0 with embedded BSD notices):
  https://github.com/ebitengine/purego/tree/v0.11.2

The bundled mpv build enables GPL features. This is not an LGPL-only build.
The accompanying libmpv-build.json lists the copied native libraries.

**This notice is a development provenance record, not a completed public release
license package or source offer. Do not publish/distribute this application build
as a release until the complete actual dependency licenses, copyright notices,
corresponding sources/build changes, and application licensing have been audited
and packaged. A generic upstream link alone does not complete GPL compliance.**
