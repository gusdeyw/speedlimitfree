Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!include "Sections.nsh"
!include "FileFunc.nsh"
!ifndef VERSION
  !define VERSION "0.3.0"
!endif
Name "SpeedLimitFree"
OutFile "${OUTPUT}"
InstallDir "$PROGRAMFILES64\SpeedLimitFree"
!ifdef FIXTURE
  RequestExecutionLevel user
!else
  RequestExecutionLevel admin
!endif
ManifestDPIAware true
SetCompressor /SOLID lzma
ShowInstDetails show
ShowUninstDetails show
VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=1033 "ProductName" "SpeedLimitFree Setup"
VIAddVersionKey /LANG=1033 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=1033 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=1033 "FileDescription" "SpeedLimitFree installer and full uninstaller"
VIAddVersionKey /LANG=1033 "CompanyName" "gusdeyw"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Copyright (c) 2026 gusdeyw"
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING
!define MUI_CUSTOMFUNCTION_GUIINIT SetupGuiInit
!define MUI_WELCOMEPAGE_TEXT "Install SpeedLimitFree and its background traffic service.$\r$\n$\r$\nSetup closes the installed desktop during updates and keeps your saved limits.$\r$\n$\r$\nUninstalling removes the app and all its saved rules, settings, logs, and caches."
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchDesktop
!define MUI_FINISHPAGE_RUN_TEXT "Open SpeedLimitFree"
!define MUI_FINISHPAGE_TEXT "SpeedLimitFree is installed. Its background service keeps limits active when you close the desktop.$\r$\n$\r$\nUse Settings for Start, Stop, Restart, or Repair."
!insertmacro MUI_PAGE_FINISH
!define MUI_UNCONFIRMPAGE_TEXT_TOP "This removes SpeedLimitFree and permanently deletes all its saved limits, settings, logs, browser data, and caches.$\r$\n$\r$\nShared Windows components used by other applications remain installed."
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "English"

Var StartupChoice
Var SetupMutex
Function .onInit
  ${IfNot} ${IsNativeAMD64}
    MessageBox MB_OK|MB_ICONSTOP "This build requires x64 Windows." /SD IDOK
    SetErrorLevel 1
    Quit
  ${EndIf}
  SetRegView 64
  !ifdef FIXTURE
    StrCpy $INSTDIR "${FIXTURE}\installed\SpeedLimitFree"
  !else
    StrCpy $INSTDIR "$PROGRAMFILES64\SpeedLimitFree"
    ReadRegStr $0 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion" "CurrentBuildNumber"
    ${If} $0 < 22000
      MessageBox MB_OK|MB_ICONSTOP "This build targets Windows 11 x64." /SD IDOK
      SetErrorLevel 1
      Quit
    ${EndIf}
  !endif
  System::Call 'kernel32::CreateMutex(p 0, i 0, t "Global\SpeedLimitFree.Installer") p.r0 ?e'
  Pop $1
  StrCpy $SetupMutex $0
  ${If} $1 = 183
    MessageBox MB_OK|MB_ICONSTOP "Another SpeedLimitFree installer is open." /SD IDOK
    SetErrorLevel 1
    Quit
  ${EndIf}
  StrCpy $StartupChoice 0
  !ifdef FIXTURE
    ReadRegDWORD $0 HKCU "${FIXTURE_KEY}" "TrayStartup"
  !else
    ReadRegDWORD $0 HKLM "SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\SpeedLimitFree" "TrayStartup"
  !endif
  ${If} $0 = 1
    StrCpy $StartupChoice 1
  ${EndIf}
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/TRAY=" $1
  ${IfNot} ${Errors}
    ${If} $1 == "0"
    ${OrIf} $1 == "1"
      StrCpy $StartupChoice $1
    ${Else}
      MessageBox MB_OK|MB_ICONSTOP "Use /TRAY=0 or /TRAY=1." /SD IDOK
      SetErrorLevel 1
      Quit
    ${EndIf}
  ${EndIf}
FunctionEnd

Function un.onInit
  SetRegView 64
  !ifdef FIXTURE
    StrCpy $INSTDIR "${FIXTURE}\installed\SpeedLimitFree"
  !else
    StrCpy $INSTDIR "$PROGRAMFILES64\SpeedLimitFree"
  !endif
FunctionEnd

Section "Application and traffic service (required)" Core
  SectionIn RO
  InitPluginsDir
  SetOutPath "$PLUGINSDIR\payload"
  File "${PAYLOAD}\SpeedLimitFree.exe"
  File "${PAYLOAD}\SpeedLimitFree-service.exe"
  File "${PAYLOAD}\limiter-debug.exe"
  File "${PAYLOAD}\WinDivert.dll"
  File "${PAYLOAD}\WinDivert64.sys"
  File "${PAYLOAD}\install-service.ps1"
  File "${PAYLOAD}\manage-service.ps1"
  File "${PAYLOAD}\service-common.ps1"
  File "${PAYLOAD}\uninstall-service.ps1"
  File "${PAYLOAD}\installer-actions.ps1"
  File "${PAYLOAD}\installer-common.ps1"
  File "${PAYLOAD}\installer-native.cs"
  File "${PAYLOAD}\START_HERE.md"
  File "${PAYLOAD}\THIRD_PARTY_NOTICES.md"
  File "${PAYLOAD}\LICENSE"
  File "${PAYLOAD}\distribution.json"
  File "${BOOTSTRAPPER}"
  WriteUninstaller "$PLUGINSDIR\payload\Uninstall.exe"
SectionEnd

Section /o "Open in the tray when I sign in" TrayStartup
SectionEnd

Section -Install
  IfSilent startup_ready
  StrCpy $StartupChoice 0
  ${If} ${SectionIsSelected} ${TrayStartup}
    StrCpy $StartupChoice 1
  ${EndIf}
  startup_ready:
  DetailPrint "Installing files and configuring the traffic service..."
  nsExec::ExecToStack '"$WINDIR\sysnative\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$PLUGINSDIR\payload\installer-actions.ps1" -Action install -Source "$PLUGINSDIR\payload" -Version "${VERSION}" -Startup $StartupChoice'
  Pop $0
  Pop $1
  DetailPrint "$1"
  ${If} $0 != 0
    MessageBox MB_OK|MB_ICONSTOP "Setup did not complete.$\r$\n$\r$\n$1" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
SectionEnd

Function SetupGuiInit
  ${If} $StartupChoice = 1
    !insertmacro SelectSection ${TrayStartup}
  ${EndIf}
FunctionEnd

Function LaunchDesktop
  nsExec::ExecToStack '"$WINDIR\sysnative\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\installer-actions.ps1" -Action launch'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_OK "Installation is complete. Open SpeedLimitFree from the Start menu." /SD IDOK
  ${EndIf}
FunctionEnd

Section "Uninstall"
  InitPluginsDir
  SetOutPath "$PLUGINSDIR\cleanup"
  File "${PAYLOAD}\installer-actions.ps1"
  File "${PAYLOAD}\installer-common.ps1"
  File "${PAYLOAD}\installer-native.cs"
  File "${PAYLOAD}\service-common.ps1"
  nsExec::ExecToStack '"$WINDIR\sysnative\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$PLUGINSDIR\cleanup\installer-actions.ps1" -Action uninstall'
  Pop $0
  Pop $1
  DetailPrint "$1"
  ${If} $0 != 0
    MessageBox MB_OK|MB_ICONSTOP "Removal did not complete. Retry after resolving this problem:$\r$\n$\r$\n$1" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${Core} "The desktop, automatic Windows traffic service, and driver files. WebView2 is installed from Microsoft if missing."
  !insertmacro MUI_DESCRIPTION_TEXT ${TrayStartup} "Start the desktop hidden in the tray for the application owner's account. The service starts automatically regardless of this choice."
!insertmacro MUI_FUNCTION_DESCRIPTION_END
