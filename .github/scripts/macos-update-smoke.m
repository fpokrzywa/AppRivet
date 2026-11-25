#import <AppKit/AppKit.h>
#import <Sparkle/SPUUpdater.h>
#import <Sparkle/SUAppcastItem.h>
#import <Sparkle/SUUpdatePermissionResponse.h>

#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

static void fail(NSString *message) __attribute__((noreturn));

static void fail(NSString *message)
{
    fprintf(stderr, "Sparkle smoke failure: %s\n", message.UTF8String);
    exit(EXIT_FAILURE);
}

@interface SparkleSmokeDriver : NSObject <NSApplicationDelegate, SPUUserDriver>
@end

@implementation SparkleSmokeDriver {
    SPUUpdater *_updater;
    NSString *_expectedVersion;
    NSString *_expectedBuild;
    NSString *_expectedDownloadURL;
}

- (void)applicationDidFinishLaunching:(__unused NSNotification *)notification
{
    NSUserDefaults *arguments = [NSUserDefaults standardUserDefaults];
    _expectedVersion = [arguments stringForKey:@"SparkleSmokeVersion"];
    _expectedBuild = [arguments stringForKey:@"SparkleSmokeBuild"];
    _expectedDownloadURL = [arguments stringForKey:@"SparkleSmokeURL"];
    if (_expectedVersion.length == 0 || _expectedBuild.length == 0 || _expectedDownloadURL.length == 0) {
        fail(@"The runner must identify the expected release.");
    }

    fprintf(stderr, "Sparkle smoke fixture PID %d\n", getpid());
    NSBundle *bundle = [NSBundle mainBundle];
    _updater = [[SPUUpdater alloc] initWithHostBundle:bundle applicationBundle:bundle userDriver:self delegate:nil];
    NSError *error = nil;
    if (![_updater startUpdater:&error]) {
        fail([NSString stringWithFormat:@"Could not start the packaged updater: %@", error]);
    }
    if (![_updater.feedURL.absoluteString isEqualToString:[bundle objectForInfoDictionaryKey:@"SUFeedURL"]]) {
        fail(@"Sparkle is using a different URL than the packaged appcast.");
    }
    [_updater checkForUpdates];
}

- (void)applicationWillTerminate:(__unused NSNotification *)notification
{
    fprintf(stderr, "Sparkle smoke: fixture received the quit request.\n");
}

- (void)showUpdatePermissionRequest:(__unused SPUUpdatePermissionRequest *)request
                              reply:(void (^)(SUUpdatePermissionResponse *))reply
{
    reply([[SUUpdatePermissionResponse alloc] initWithAutomaticUpdateChecks:NO sendSystemProfile:NO]);
}

- (void)showUserInitiatedUpdateCheckWithCancellation:(__unused void (^)(void))cancellation
{
    fprintf(stderr, "Sparkle smoke: checking the production appcast.\n");
}

- (void)showUpdateFoundWithAppcastItem:(SUAppcastItem *)item
                                state:(SPUUserUpdateState *)state
                                reply:(void (^)(SPUUserUpdateChoice))reply
{
    if (!state.userInitiated || item.informationOnlyUpdate ||
        ![item.displayVersionString isEqualToString:_expectedVersion] ||
        ![item.versionString isEqualToString:_expectedBuild] ||
        ![item.fileURL.absoluteString isEqualToString:_expectedDownloadURL]) {
        reply(SPUUserUpdateChoiceDismiss);
        fail([NSString stringWithFormat:@"Unexpected appcast item %@ (build %@) at %@.",
                                        item.displayVersionString, item.versionString, item.fileURL]);
    }
    fprintf(stderr, "Sparkle smoke: approved download of %s (build %s).\n",
            item.displayVersionString.UTF8String, item.versionString.UTF8String);
    reply(SPUUserUpdateChoiceInstall);
}

- (void)showUpdateReleaseNotesWithDownloadData:(__unused SPUDownloadData *)data {}

- (void)showUpdateReleaseNotesFailedToDownloadWithError:(NSError *)error
{
    fprintf(stderr, "Sparkle smoke: release notes were unavailable (%s).\n", error.localizedDescription.UTF8String);
}

- (void)showUpdateNotFoundWithError:(NSError *)error acknowledgement:(void (^)(void))acknowledgement
{
    acknowledgement();
    fail([NSString stringWithFormat:@"No installable update: %@", error]);
}

- (void)showUpdaterError:(NSError *)error acknowledgement:(void (^)(void))acknowledgement
{
    acknowledgement();
    fail([NSString stringWithFormat:@"Updater error: %@", error]);
}

- (void)showDownloadInitiatedWithCancellation:(__unused void (^)(void))cancellation
{
    fprintf(stderr, "Sparkle smoke: downloading the update.\n");
}

- (void)showDownloadDidReceiveExpectedContentLength:(uint64_t)length
{
    fprintf(stderr, "Sparkle smoke: expected download size is %" PRIu64 " bytes.\n", length);
}

- (void)showDownloadDidReceiveDataOfLength:(__unused uint64_t)length {}

- (void)showDownloadDidStartExtractingUpdate
{
    fprintf(stderr, "Sparkle smoke: staging the downloaded update.\n");
}

- (void)showExtractionReceivedProgress:(__unused double)progress {}

- (void)showReadyToInstallAndRelaunch:(void (^)(SPUUserUpdateChoice))reply
{
    fprintf(stderr, "Sparkle smoke: approved install and relaunch.\n");
    reply(SPUUserUpdateChoiceInstall);
}

- (void)showInstallingUpdateWithApplicationTerminated:(BOOL)terminated
                          retryTerminatingApplication:(__unused void (^)(void))retry
{
    fprintf(stderr, "Sparkle smoke: installation started; fixture %s.\n", terminated ? "has quit" : "is still running");
}

- (void)showUpdateInstalledAndRelaunched:(BOOL)relaunched acknowledgement:(void (^)(void))acknowledgement
{
    fprintf(stderr, "Sparkle smoke: installer reported relaunch=%d.\n", relaunched);
    acknowledgement();
}

- (void)dismissUpdateInstallation {}

@end

int main(void)
{
    @autoreleasepool {
        NSApplication *application = [NSApplication sharedApplication];
        SparkleSmokeDriver *driver __attribute__((objc_precise_lifetime)) = [SparkleSmokeDriver new];
        application.delegate = driver;
        [application run];
    }
    return EXIT_SUCCESS;
}
