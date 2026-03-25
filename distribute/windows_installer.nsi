; Typstify Windows Installer Script
; ---------------------------------

!define APPNAME "Typstify"
!define APPVERSION "v1.6.0"
!define INSTALLDIR "$PROGRAMFILES64\${APPNAME}"

Outfile "TypstifyInstaller.exe"
InstallDir "${INSTALLDIR}"
RequestExecutionLevel admin

;--------------------------------
; Installer Sections
Section "Install Typstify" 
    ; Force close the app if it is running to prevent "Error opening file for writing"
    ; /F = Forcefully terminate
    ; /IM = Image Name (process name)
    nsExec::Exec 'taskkill /F /IM "typstify.exe"'
    
    ; Also kill dependencies if they run as standalone processes that might be locked
    nsExec::Exec 'taskkill /F /IM "typstify.exe"'
    nsExec::Exec 'taskkill /F /IM "typst.exe"'
    nsExec::Exec 'taskkill /F /IM "tinymist.exe"'

    ; Pause briefly to ensure Windows releases the file locks
    Sleep 1000

    ; Set Installation Path
    SetOutPath "${INSTALLDIR}"

    ; Copy Main Executable
    File "typstify.exe"

    ; Copy Dependency (Typst & Tinymist Binary)
    File "assets\windows-x86_64-msvc\typst.exe"
    File "assets\windows-x86_64-msvc\tinymist.exe"

    ; Create Uninstaller
    WriteUninstaller "$INSTDIR\Uninstall.exe"

    ; Add Start Menu Shortcut
    CreateShortcut "$SMPROGRAMS\${APPNAME}.lnk" "$INSTDIR\typstify.exe"

    ; Add Desktop Shortcut
    CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\typstify.exe"

SectionEnd

;--------------------------------
; Uninstaller Section
Section "Uninstall"

    ; Remove Files
    Delete "$INSTDIR\typstify.exe"
    Delete "$INSTDIR\typst.exe"
    Delete "$INSTDIR\tinymist.exe"
    Delete "$INSTDIR\Uninstall.exe"

    ; Remove Shortcuts
    Delete "$SMPROGRAMS\${APPNAME}.lnk"
    Delete "$DESKTOP\${APPNAME}.lnk"

    ; Remove Directory
    RMDir "$INSTDIR"

SectionEnd

;--------------------------------
; Uninstaller Configuration
Section "Install Uninstaller"
    WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd
