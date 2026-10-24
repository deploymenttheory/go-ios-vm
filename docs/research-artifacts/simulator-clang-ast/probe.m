#import <TargetConditionals.h>
#import <UIKit/UIKit.h>
#import <Metal/Metal.h>
#import <QuartzCore/CAMetalLayer.h>
void VMResearchProbe(UIWindow *window, CAMetalLayer *layer) {
    window.rootViewController = nil;
    layer.device = MTLCreateSystemDefaultDevice();
    layer.drawableSize = (CGSize){640, 480};
}
