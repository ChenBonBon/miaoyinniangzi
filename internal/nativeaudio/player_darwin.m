#import <AVFoundation/AVFoundation.h>
#import "player.h"
#include <math.h>

@interface MYFramePlayer : NSObject
@property(nonatomic,strong) AVAudioEngine *engine;
@property(nonatomic,strong) AVAudioPlayerNode *node;
@property(nonatomic,strong) AVAudioFile *file;
@property(nonatomic,strong) dispatch_queue_t queue;
@property AVAudioFramePosition base;
@property AVAudioFramePosition pausedFrame;
@property NSUInteger generation;
@property BOOL running;
@property BOOL ended;
@property BOOL scheduled;
@end
@implementation MYFramePlayer
- (AVAudioFramePosition)frame {
    if (!self.running) return self.pausedFrame;
    AVAudioTime *render = self.node.lastRenderTime;
    AVAudioTime *time = render ? [self.node playerTimeForNodeTime:render] : nil;
    AVAudioFramePosition frame = self.base + (time ? MAX(0, time.sampleTime) : 0);
    return MIN(frame, self.file.length);
}
- (void)schedule:(AVAudioFramePosition)frame {
    self.generation++;
    self.running = NO;
    [self.node stop];
    self.base = frame;
    self.pausedFrame = frame;
    self.ended = NO;
    self.scheduled = YES;
    NSUInteger generation = self.generation;
    AVAudioFrameCount count = (AVAudioFrameCount)(self.file.length-frame);
    __weak MYFramePlayer *weak = self;
    [self.node scheduleSegment:self.file startingFrame:frame frameCount:count atTime:nil completionCallbackType:AVAudioPlayerNodeCompletionDataPlayedBack completionHandler:^(AVAudioPlayerNodeCompletionCallbackType type) {
        MYFramePlayer *strong = weak;
        if (!strong) return;
        dispatch_async(strong.queue, ^{
            if (strong.generation != generation) return;
            strong.running = NO;
            strong.scheduled = NO;
            strong.ended = YES;
            strong.pausedFrame = strong.file.length;
        });
    }];
}
@end

void *miaoyin_audio_open(const char *path) {
    @autoreleasepool {
        @try {
            MYFramePlayer *p = [MYFramePlayer new];
            p.queue = dispatch_queue_create("com.miaoyinniangzi.sample-player", DISPATCH_QUEUE_SERIAL);
            NSError *error = nil;
            p.file = [[AVAudioFile alloc] initForReading:[NSURL fileURLWithPath:[NSString stringWithUTF8String:path]] error:&error];
            if (!p.file || p.file.length <= 0 || p.file.length > UINT32_MAX || p.file.processingFormat.sampleRate <= 0 || p.file.processingFormat.sampleRate > 48000 || p.file.processingFormat.channelCount > 2) return NULL;
            p.engine = [AVAudioEngine new];p.node = [AVAudioPlayerNode new];
            [p.engine attachNode:p.node];
            [p.engine connect:p.node to:p.engine.mainMixerNode format:p.file.processingFormat];
            [p.engine prepare];
            [p schedule:0];
            return (__bridge_retained void *)p;
        } @catch (NSException *exception) { return NULL; }
    }
}
int miaoyin_audio_play(void *handle) {
    MYFramePlayer *p = (__bridge MYFramePlayer *)handle;if (!p) return 0;
    __block int ok = 0;
    dispatch_sync(p.queue, ^{
        @try {
            if (!p.scheduled || p.ended) [p schedule:0];
            NSError *error = nil;
            if (!p.engine.running && ![p.engine startAndReturnError:&error]) return;
            [p.node play];p.running = YES;ok = 1;
        } @catch (NSException *exception) {}
    });return ok;
}
void miaoyin_audio_pause(void *handle) {
    MYFramePlayer *p = (__bridge MYFramePlayer *)handle;if (!p) return;
    dispatch_sync(p.queue, ^{p.pausedFrame = [p frame];[p.node pause];p.running = NO;});
}
int miaoyin_audio_seek(void *handle,double seconds) {
    MYFramePlayer *p = (__bridge MYFramePlayer *)handle;if (!p || !isfinite(seconds)) return 0;
    __block int ok = 0;
    dispatch_sync(p.queue, ^{
        @try {
            BOOL running = p.running;
            double rate = p.file.processingFormat.sampleRate;
            AVAudioFramePosition frame = (AVAudioFramePosition)llround(MAX(0,MIN(seconds, (double)(p.file.length-1)/rate))*rate);
            [p schedule:MIN(frame,p.file.length-1)];
            if (running) {[p.node play];p.running = YES;}ok = 1;
        } @catch (NSException *exception) {}
    });return ok;
}
void miaoyin_audio_volume(void *handle,double volume) {
    MYFramePlayer *p = (__bridge MYFramePlayer *)handle;if (!p) return;
    dispatch_sync(p.queue, ^{p.node.volume = (float)MAX(0,MIN(1,volume));});
}
MiaoyinAudioState miaoyin_audio_state(void *handle) {
    MYFramePlayer *p = (__bridge MYFramePlayer *)handle;__block MiaoyinAudioState state = {0};if (!p) return state;
    dispatch_sync(p.queue, ^{state.position = (double)[p frame]/p.file.processingFormat.sampleRate;state.duration=(double)p.file.length/p.file.processingFormat.sampleRate;state.playing=p.running&&p.node.playing&&p.engine.running;state.ended=p.ended;});return state;
}
void miaoyin_audio_close(void *handle) {
    if (!handle) return;
    MYFramePlayer *p = (__bridge_transfer MYFramePlayer *)handle;
    dispatch_sync(p.queue, ^{p.generation++;p.running=NO;[p.node stop];[p.engine stop];p.file=nil;});
}
// Decoder-level test probe. Separate file instance, never the scheduled player.
double miaoyin_audio_probe_frequency(const char *path,double seconds) {
    @autoreleasepool {
        NSError *error=nil;
        AVAudioFile *file=[[AVAudioFile alloc] initForReading:[NSURL fileURLWithPath:[NSString stringWithUTF8String:path]] error:&error];
        if (!file || seconds<0) return -1;
        file.framePosition=(AVAudioFramePosition)llround(seconds*file.processingFormat.sampleRate);
        AVAudioPCMBuffer *buffer=[[AVAudioPCMBuffer alloc] initWithPCMFormat:file.processingFormat frameCapacity:4096];
        if (![file readIntoBuffer:buffer frameCount:4096 error:&error] || !buffer.floatChannelData || buffer.frameLength<100) return -1;
        const float *samples=buffer.floatChannelData[0];unsigned crossings=0;
        for (unsigned i=1;i<buffer.frameLength;i++) if(samples[i-1]<0 && samples[i]>=0)crossings++;
        return (double)crossings*file.processingFormat.sampleRate/(buffer.frameLength-1);
    }
}
