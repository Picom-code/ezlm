; EZLM for Windows: per-user installer (no admin prompt), x64 and ARM64 in one file.
; Build: ISCC.exe /DSrcX64=<publish dir> /DSrcArm64=<publish dir> EZLM.iss

#define AppVer "0.1.11"

[Setup]
AppId={{1B744E4B-F94C-43D3-ACF7-5DF888CD773C}
AppName=EZLM
AppVersion={#AppVer}
AppPublisher=EZLM
AppPublisherURL=https://github.com/picom-code/ezlm
DefaultDirName={localappdata}\Programs\EZLM
DisableDirPage=yes
DisableProgramGroupPage=yes
DefaultGroupName=EZLM
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible or arm64
ArchitecturesInstallIn64BitMode=x64compatible or arm64
OutputBaseFilename=EZLM-Setup-{#AppVer}
OutputDir=.
SetupIconFile={#SrcX64}\Assets\EZLM.ico
UninstallDisplayIcon={app}\EZLM.exe
UninstallDisplayName=EZLM
Compression=lzma2/ultra64
SolidCompression=yes
LZMAUseSeparateProcess=yes
WizardStyle=modern
CloseApplications=yes
RestartApplications=no
; No AppMutex: when EZLM updates itself it starts this installer and then
; exits; a mutex check could catch it mid-exit and abort a silent update.
; Files still in use are handled by CloseApplications (Restart Manager).
MinVersion=10.0.19041

[Tasks]
Name: "desktopicon"; Description: "Put an EZLM icon on the desktop"; GroupDescription: "Shortcuts:"

[Files]
Source: "{#SrcX64}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs; Check: not IsArm64
Source: "{#SrcArm64}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs; Check: IsArm64

[InstallDelete]
; files from older builds that would otherwise linger
Type: filesandordirs; Name: "{app}\*.xbf"

[Icons]
Name: "{autoprograms}\EZLM"; Filename: "{app}\EZLM.exe"
Name: "{autodesktop}\EZLM"; Filename: "{app}\EZLM.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\EZLM.exe"; Description: "Open EZLM"; Flags: nowait postinstall skipifsilent
; After an in-app update (/UPDATE=1), open EZLM again even though setup ran silently.
Filename: "{app}\EZLM.exe"; Flags: nowait; Check: IsUpdate

[Registry]
; The login entry is written by EZLM when you set up a model; remove it on uninstall.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueName: "EZLM"; ValueType: none; Flags: uninsdeletevalue dontcreatekey

[Code]
var
  RemoveModels: Boolean;
  Installed: Boolean;

function IsUpdate(): Boolean;
begin
  Result := ExpandConstant('{param:UPDATE|0}') = '1';
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    Installed := True;
end;

{ An in-app update closed EZLM before running setup. If setup stops early
  (a file stayed in use), open the installed EZLM again so the user is never
  left without it; they can retry the update from Settings. }
procedure DeinitializeSetup();
var
  Code: Integer;
begin
  if IsUpdate() and not Installed then
  try
    if FileExists(ExpandConstant('{app}\EZLM.exe')) then
      ExecAsOriginalUser(ExpandConstant('{app}\EZLM.exe'), '', '', SW_SHOW, ewNoWait, Code);
  except
    { setup stopped before the install folder was known: nothing to reopen }
  end;
end;

function InitializeUninstall(): Boolean;
begin
  { SuppressibleMsgBox: a silent uninstall (/SUPPRESSMSGBOXES) answers No and keeps the models. }
  RemoveModels := SuppressibleMsgBox('Also delete the AI models EZLM downloaded, and its settings and keys?' + #13#10#13#10 +
    'Choose No to keep them, so reinstalling EZLM doesn''t download them again.', mbConfirmation, MB_YESNO or MB_DEFBUTTON2, IDNO) = IDYES;
  Result := True;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Code: Integer;
begin
  if CurUninstallStep = usUninstall then
  begin
    { Stop the model server (only EZLM's own), and optionally remove its data. }
    if RemoveModels then
      Exec(ExpandConstant('{app}\Engine\ezlm.exe'), 'uninstall', '', SW_HIDE, ewWaitUntilTerminated, Code)
    else
      Exec(ExpandConstant('{app}\Engine\ezlm.exe'), 'stop --for-uninstall', '', SW_HIDE, ewWaitUntilTerminated, Code);
  end;
end;
