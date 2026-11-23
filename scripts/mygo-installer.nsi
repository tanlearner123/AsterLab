!include "MUI2.nsh"

!ifndef VERSION
  !define VERSION "0.1.0-unsigned"
!endif

!define MUI_ICON "..\prototypes\mygo-run\assets\asterlab.ico"
!define MUI_UNICON "..\prototypes\mygo-run\assets\asterlab.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\mygo-run.exe"

Name "AsterLab (${VERSION})"
OutFile "..\release\AsterLab-MyGo-${VERSION}-win-x64-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\AsterLab MyGo"
InstallDirRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AsterLabMyGo" "InstallLocation"
RequestExecutionLevel user
SetCompressor /SOLID lzma
SetOverwrite on

!if /FileExists "..\prototypes\mygo-run\dist\models\nl-router\router.int8.onnx"
  !if /FileExists "..\prototypes\mygo-run\dist\onnxruntime.dll"
    !define HAS_NL_MODEL 1
  !endif
!endif

!ifdef HAS_NL_MODEL
  !insertmacro MUI_PAGE_COMPONENTS
!endif
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "!AsterLab Core (Required)" SecCore
    SectionIn RO
    SetShellVarContext current
    SetOutPath "$INSTDIR"

    ; Pre-flight write permission check before killing or unpacking files.
    ClearErrors
    FileOpen $1 "$INSTDIR\.asterlab-install-probe.tmp" w
    IfErrors 0 probeWritableOk
        MessageBox MB_ICONSTOP|MB_OK "Cannot write to target folder:$\r$\n$INSTDIR$\r$\n$\r$\nPlease select a user-writable folder (such as %LOCALAPPDATA%\Programs\AsterLab MyGo) or run as Administrator."
        Abort
    probeWritableOk:
    FileClose $1
    Delete "$INSTDIR\.asterlab-install-probe.tmp"

    ; Electron-Builder / NSIS mature in-place upgrade flow (pure Win32 + NSIS, zero WOW64 PowerShell traps):
    ; 1) Ask running AsterLab to close gracefully, then force-terminate remaining locked processes.
    IfFileExists "$INSTDIR\mygo-run.exe" 0 doneClose
        DetailPrint "Closing running AsterLab processes..."
        nsExec::Exec 'taskkill.exe /IM "mygo-run.exe"'
        Sleep 700
        nsExec::Exec 'taskkill.exe /F /T /IM "mygo-run.exe"'
        nsExec::Exec 'taskkill.exe /F /T /IM "asterlab.exe"'
        Sleep 400
    doneClose:

    ; 2) Clear read-only attributes and atomically move aside old binaries (.old) before unpacking.
    ;    On Windows, Rename succeeds even if Explorer/AV holds a brief handle on an .exe.
    Delete "$INSTDIR\mygo-run.exe.old"
    Delete "$INSTDIR\asterlab.exe.old"
    Delete "$INSTDIR\uninstall.exe.old"
    IfFileExists "$INSTDIR\mygo-run.exe" 0 skipRenameExe
        SetFileAttributes "$INSTDIR\mygo-run.exe" NORMAL
        ClearErrors
        Rename "$INSTDIR\mygo-run.exe" "$INSTDIR\mygo-run.exe.old"
        IfErrors 0 skipRenameExe
            ; Wait briefly if OS is still flushing process handles
            StrCpy $0 0
            waitLockLoop:
                IntCmp $0 20 skipRenameExe skipRenameExe 0
                Sleep 250
                IntOp $0 $0 + 1
                ClearErrors
                Rename "$INSTDIR\mygo-run.exe" "$INSTDIR\mygo-run.exe.old"
                IfErrors waitLockLoop skipRenameExe
    skipRenameExe:
    IfFileExists "$INSTDIR\asterlab.exe" 0 skipRenameCli
        SetFileAttributes "$INSTDIR\asterlab.exe" NORMAL
        Rename "$INSTDIR\asterlab.exe" "$INSTDIR\asterlab.exe.old"
    skipRenameCli:
    IfFileExists "$INSTDIR\uninstall.exe" 0 skipRenameUninst
        SetFileAttributes "$INSTDIR\uninstall.exe" NORMAL
        Rename "$INSTDIR\uninstall.exe" "$INSTDIR\uninstall.exe.old"
    skipRenameUninst:

    File "..\prototypes\mygo-run\dist\mygo-run.exe"
    File "..\prototypes\mygo-run\dist\asterlab.exe"
    File "..\prototypes\mygo-run\dist\BUILD_INFO.txt"
    File "..\prototypes\mygo-run\assets\asterlab.ico"
    Delete "$INSTDIR\AsterLab-MyGo.bat"
    Delete "$INSTDIR\mygo-run.exe.old"
    Delete "$INSTDIR\asterlab.exe.old"
    Delete "$INSTDIR\uninstall.exe.old"

    CreateDirectory "$SMPROGRAMS\AsterLab MyGo"
    CreateShortcut "$SMPROGRAMS\AsterLab MyGo\AsterLab MyGo.lnk" "$INSTDIR\mygo-run.exe" "" "$INSTDIR\asterlab.ico" 0
    CreateShortcut "$DESKTOP\AsterLab MyGo.lnk" "$INSTDIR\mygo-run.exe" "" "$INSTDIR\asterlab.ico" 0

    WriteUninstaller "$INSTDIR\uninstall.exe"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AsterLabMyGo" "DisplayName" "AsterLab (${VERSION})"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AsterLabMyGo" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AsterLabMyGo" "InstallLocation" "$INSTDIR"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AsterLabMyGo" "DisplayIcon" "$INSTDIR\asterlab.ico"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AsterLabMyGo" "DisplayVersion" "${VERSION}"
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AsterLabMyGo" "NoModify" 1
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AsterLabMyGo" "NoRepair" 1
    ; User-level asterlab:// protocol handler (CLOSE-07): per-user, no
    ; elevation. The OS hands the raw URL to the exe as one quoted argument;
    ; strict route parsing happens in-process (internal/state), never shell
    ; execution.
    WriteRegStr HKCU "Software\Classes\asterlab" "" "URL:AsterLab Run Link"
    WriteRegStr HKCU "Software\Classes\asterlab" "URL Protocol" ""
    WriteRegStr HKCU "Software\Classes\asterlab\DefaultIcon" "" "$\"$INSTDIR\asterlab.ico$\",0"
    WriteRegStr HKCU "Software\Classes\asterlab\shell\open\command" "" "$\"$INSTDIR\mygo-run.exe$\" $\"%1$\""
    ; Register .asterml single-source file association so double-clicking opens in AsterLab.
    WriteRegStr HKCU "Software\Classes\.asterml" "" "AsterLab.AstermlFile"
    WriteRegStr HKCU "Software\Classes\AsterLab.AstermlFile" "" "AsterLab Experiment Document (.asterml)"
    WriteRegStr HKCU "Software\Classes\AsterLab.AstermlFile\DefaultIcon" "" "$\"$INSTDIR\asterlab.ico$\",0"
    WriteRegStr HKCU "Software\Classes\AsterLab.AstermlFile\shell\open\command" "" "$\"$INSTDIR\mygo-run.exe$\" $\"%1$\""
SectionEnd

!ifdef HAS_NL_MODEL
Section "Offline AI Semantic Router (~23 MB)" SecNLModel
    SetOutPath "$INSTDIR"
    Delete "$INSTDIR\onnxruntime.dll.old"
    IfFileExists "$INSTDIR\onnxruntime.dll" 0 skipRenameOrt
        SetFileAttributes "$INSTDIR\onnxruntime.dll" NORMAL
        Rename "$INSTDIR\onnxruntime.dll" "$INSTDIR\onnxruntime.dll.old"
    skipRenameOrt:
    File "..\prototypes\mygo-run\dist\onnxruntime.dll"
    Delete "$INSTDIR\onnxruntime.dll.old"

    SetOutPath "$INSTDIR\models\nl-router"
    File "..\prototypes\mygo-run\dist\models\nl-router\manifest.json"
    File "..\prototypes\mygo-run\dist\models\nl-router\router.int8.onnx"
    File "..\prototypes\mygo-run\dist\models\nl-router\vocab.txt"
    !if /FileExists "..\prototypes\mygo-run\dist\models\nl-router\schemas.json"
        File "..\prototypes\mygo-run\dist\models\nl-router\schemas.json"
    !endif
SectionEnd
!endif

Section "Uninstall"
    SetShellVarContext current
    Delete "$INSTDIR\mygo-run.exe"
    Delete "$INSTDIR\asterlab.exe"
    Delete "$INSTDIR\BUILD_INFO.txt"
    Delete "$INSTDIR\asterlab.ico"
    Delete "$INSTDIR\onnxruntime.dll"
    Delete "$INSTDIR\onnxruntime.dll.old"
    Delete "$INSTDIR\models\nl-router\manifest.json"
    Delete "$INSTDIR\models\nl-router\router.int8.onnx"
    Delete "$INSTDIR\models\nl-router\vocab.txt"
    Delete "$INSTDIR\models\nl-router\schemas.json"
    RMDir "$INSTDIR\models\nl-router"
    RMDir "$INSTDIR\models"
    Delete "$INSTDIR\AsterLab-MyGo.bat"
    Delete "$INSTDIR\uninstall.exe"
    RMDir "$INSTDIR"
    Delete "$SMPROGRAMS\AsterLab MyGo\AsterLab MyGo.lnk"
    RMDir "$SMPROGRAMS\AsterLab MyGo"
    Delete "$DESKTOP\AsterLab MyGo.lnk"
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\AsterLabMyGo"
    DeleteRegKey HKCU "Software\Classes\asterlab"
    DeleteRegKey HKCU "Software\Classes\.asterml"
    DeleteRegKey HKCU "Software\Classes\AsterLab.AstermlFile"
SectionEnd
