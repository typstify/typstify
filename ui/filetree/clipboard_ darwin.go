package filetree

/*
#cgo darwin CFLAGS: -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#import <stdlib.h>

// Returns a newline-separated list of absolute file paths from the clipboard
char* GetMacOSClipboardFiles() {
    NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
    NSArray *classes = @[[NSURL class]];
    NSDictionary *options = @{};

    // Check if the clipboard contains actual file URLs
    if ([pasteboard canReadObjectForClasses:classes options:options]) {
        NSArray *urls = [pasteboard readObjectsForClasses:classes options:options];
        NSMutableString *result = [NSMutableString string];

        for (NSURL *url in urls) {
            if ([url isFileURL]) {
                [result appendFormat:@"%@\n", [url path]];
            }
        }
        return strdup([result UTF8String]);
    }
    return NULL;
}
*/
import "C"
import (
	"strings"
	"unsafe"
)

// ReadMacClipboardFiles reads actual absolute paths from the macOS pasteboard.
//
// When coping files in Finder using Cmd+C, macOS doesn't just put "text" on the
// clipboard (the NSPasteboard). It puts multiple representations of those files
// on the clipboard simultaneously. The two most important ones are:
//
//	public.file-url: The actual absolute paths to the files.
//	public.utf8-plain-text: A simple text fallback which are just raw filenames separated by space.
//
// To simplify the copy/paste process without using 'Copy as Pathname', we can use
// this function to bypass Gio's clipboard read command.
func ReadClipboardFiles() []string {
	cStr := C.GetMacOSClipboardFiles()
	if cStr == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(cStr))

	goStr := C.GoString(cStr)
	goStr = strings.TrimSpace(goStr)
	if goStr == "" {
		return nil
	}

	return strings.Split(goStr, "\n")
}
