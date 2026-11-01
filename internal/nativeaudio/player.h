#include <stdint.h>
typedef struct { double position; double duration; int playing; int ended; } MiaoyinAudioState;
void *miaoyin_audio_open(const char *path);
int miaoyin_audio_play(void *handle);
void miaoyin_audio_pause(void *handle);
int miaoyin_audio_seek(void *handle,double seconds);
void miaoyin_audio_volume(void *handle,double volume);
MiaoyinAudioState miaoyin_audio_state(void *handle);
void miaoyin_audio_close(void *handle);
double miaoyin_audio_probe_frequency(const char *path,double seconds);
