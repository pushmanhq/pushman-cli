#ifndef Version
  #error Version is required
#endif
#if Ver != EncodeVer(6, 7, 3)
  #error Inno Setup 6.7.3 is required
#endif
#ifndef Executable
  #error Executable is required
#endif
#ifndef Architecture
  #error Architecture is required
#endif
#ifndef OutputDirectory
  #error OutputDirectory is required
#endif
#ifndef AppIdentity
  #define AppIdentity "222c0b32-df1d-468b-a624-913a96355115"
#endif
#ifndef OwnerKey
  #define OwnerKey "Software\Pushman\Installer"
#endif

[Setup]
AppId={#AppIdentity}
AppName=Pushman CLI
AppVersion={#Version}
AppPublisher=Pushman
AppPublisherURL=https://github.com/pushmanhq/pushman
AppSupportURL=https://github.com/pushmanhq/pushman-cli/issues
AppUpdatesURL=https://github.com/pushmanhq/pushman-cli/releases
DefaultDirName={localappdata}\Programs\Pushman
DefaultGroupName=Pushman
PrivilegesRequired=lowest
DisableProgramGroupPage=yes
AllowNoIcons=yes
DisableDirPage=auto
WizardStyle=modern dynamic windows11
WizardSmallImageFile=..\..\docs\assets\pushman-icon.png
WizardSmallImageFileDynamicDark=..\..\docs\assets\pushman-icon.png
LicenseFile=..\..\LICENSE
OutputDir={#OutputDirectory}
OutputBaseFilename=pushman_{#Version}_windows_{#Architecture}_setup
Compression=lzma2
SolidCompression=yes
ChangesEnvironment=yes
CloseApplications=yes
RestartApplications=no
UninstallDisplayName=Pushman CLI
UninstallDisplayIcon={app}\pushman.exe
#if Architecture == "arm64"
ArchitecturesAllowed=arm64
#else
ArchitecturesAllowed=x64compatible and not arm64
#endif

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "korean"; MessagesFile: "compiler:Languages\Korean.isl"

[Tasks]
Name: "addtopath"; Description: "{cm:AddToPath}"; Flags: checkedonce

[Files]
Source: "{#Executable}"; DestDir: "{app}"; DestName: "pushman.exe"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion

[Icons]
Name: "{group}\Pushman documentation"; Filename: "https://github.com/pushmanhq/pushman-cli#readme"

[CustomMessages]
english.AddToPath=Add Pushman to my user PATH (recommended)
korean.AddToPath=사용자 PATH에 Pushman 추가 (권장)
english.FinishedHeading=Pushman is ready
english.FinishedLabel=Open a new terminal and run "pushman login" to authorize this machine.%n%nUse "pushman help" for commands. Restart desktop MCP clients after installing or updating.
korean.FinishedHeading=Pushman 설치 완료
korean.FinishedLabel=새 터미널에서 "pushman login"을 실행해 이 컴퓨터를 인증하세요.%n%n명령 안내는 "pushman help"로 확인하세요. 설치나 업데이트 후 데스크톱 MCP 클라이언트를 다시 시작하세요.

[Code]
const
  OwnershipKey = '{#OwnerKey}';

function RegOpenKeyExW(Key: LongWord; Name: String; Options, Access: LongWord; var Handle: LongWord): LongInt;
  external 'RegOpenKeyExW@advapi32.dll stdcall';
function RegQueryValueExW(Key: LongWord; Name: String; Reserved: LongWord; var Kind: LongWord; Data: LongWord; var Size: LongWord): LongInt;
  external 'RegQueryValueExW@advapi32.dll stdcall';
function RegCloseKey(Key: LongWord): LongInt;
  external 'RegCloseKey@advapi32.dll stdcall';

function PathValueKind: LongWord;
var
  Handle, Kind, Size: LongWord;
begin
  Result := 2;
  if RegOpenKeyExW($80000001, 'Environment', 0, 1, Handle) = 0 then begin
    Size := 0;
    if RegQueryValueExW(Handle, 'Path', 0, Kind, 0, Size) = 0 then
      if Kind = 1 then Result := 1;
    RegCloseKey(Handle);
  end;
end;

procedure WriteUserPath(Value: String);
var
  Success: Boolean;
begin
  if PathValueKind = 1 then
    Success := RegWriteStringValue(HKCU, 'Environment', 'Path', Value)
  else
    Success := RegWriteExpandStringValue(HKCU, 'Environment', 'Path', Value);
  if not Success then RaiseException('Could not update the Pushman user PATH entry.');
end;

{ Pos with an ANSI literal can return a byte index for a Unicode String.
  Keep indexes in UTF-16 units for paths containing Korean or other scripts. }
function FindSeparator(Value: String; Separator: Char): Integer;
var
  Index: Integer;
begin
  Result := 0;
  for Index := 1 to Length(Value) do
    if Value[Index] = Separator then begin
      Result := Index;
      Exit;
    end;
end;

function ExpandPathVariables(Value: String): String;
var
  Variable, Expanded: String;
  StartPos, EndPos: Integer;
begin
  Result := '';
  while Value <> '' do begin
    StartPos := FindSeparator(Value, '%');
    if StartPos = 0 then begin
      Result := Result + Value;
      Exit;
    end;
    Result := Result + Copy(Value, 1, StartPos - 1);
    Delete(Value, 1, StartPos);
    EndPos := FindSeparator(Value, '%');
    if EndPos = 0 then begin
      Result := Result + '%' + Value;
      Exit;
    end;
    Variable := Copy(Value, 1, EndPos - 1);
    Expanded := GetEnv(Variable);
    if Expanded = '' then Result := Result + '%' + Variable + '%'
    else Result := Result + Expanded;
    Delete(Value, 1, EndPos);
  end;
end;

function NormalizePathEntry(Value: String): String;
begin
  Result := Trim(Value);
  if (Length(Result) >= 2) and (Result[1] = '"') and (Result[Length(Result)] = '"') then
    Result := Copy(Result, 2, Length(Result) - 2);
  Result := ExpandPathVariables(Result);
  while (Length(Result) > 3) and (Result[Length(Result)] = '\') do
    Delete(Result, Length(Result), 1);
end;

function PathContains(Value, Entry: String): Boolean;
var
  Part: String;
  Separator: Integer;
begin
  Result := False;
  repeat
    Separator := FindSeparator(Value, ';');
    if Separator = 0 then begin
      Part := Value;
      Value := '';
    end else begin
      Part := Copy(Value, 1, Separator - 1);
      Delete(Value, 1, Separator);
    end;
    if CompareText(NormalizePathEntry(Part), NormalizePathEntry(Entry)) = 0 then begin
      Result := True;
      Exit;
    end;
  until Separator = 0;
end;

function RemovePathEntry(Value, Entry: String): String;
var
  Part: String;
  Separator: Integer;
  HasPart, Removed: Boolean;
begin
  Result := '';
  HasPart := False;
  Removed := False;
  repeat
    Separator := FindSeparator(Value, ';');
    if Separator = 0 then begin
      Part := Value;
      Value := '';
    end else begin
      Part := Copy(Value, 1, Separator - 1);
      Delete(Value, 1, Separator);
    end;
    if Removed or (CompareText(NormalizePathEntry(Part), NormalizePathEntry(Entry)) <> 0) then begin
      if HasPart then Result := Result + ';';
      Result := Result + Part;
      HasPart := True;
    end else Removed := True;
  until Separator = 0;
end;

procedure RemoveOwnedPath;
var
  Entry, Value, Updated: String;
  WasMissing: Cardinal;
begin
  if RegQueryStringValue(HKCU, OwnershipKey, 'PathEntry', Entry) then begin
    if RegQueryStringValue(HKCU, 'Environment', 'Path', Value) then begin
      Updated := RemovePathEntry(Value, Entry);
      if Updated <> Value then begin
        WasMissing := 0;
        RegQueryDWordValue(HKCU, OwnershipKey, 'PathWasMissing', WasMissing);
        if (Updated = '') and (WasMissing = 1) then
          RegDeleteValue(HKCU, 'Environment', 'Path')
        else
          WriteUserPath(Updated);
      end;
    end;
    RegDeleteValue(HKCU, OwnershipKey, 'PathEntry');
    RegDeleteValue(HKCU, OwnershipKey, 'PathWasMissing');
    RegDeleteKeyIfEmpty(HKCU, OwnershipKey);
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  Entry, Owned, Value: String;
begin
  if CurStep = ssPostInstall then begin
    Entry := ExpandConstant('{app}');
    if RegQueryStringValue(HKCU, OwnershipKey, 'PathEntry', Owned) then
      if (CompareText(NormalizePathEntry(Owned), NormalizePathEntry(Entry)) <> 0) or
         (not WizardIsTaskSelected('addtopath')) then
        RemoveOwnedPath;
    if WizardIsTaskSelected('addtopath') then begin
      RegQueryStringValue(HKCU, 'Environment', 'Path', Value);
      if not PathContains(Value, Entry) then begin
        if not RegValueExists(HKCU, 'Environment', 'Path') then
          if not RegWriteDWordValue(HKCU, OwnershipKey, 'PathWasMissing', 1) then
            RaiseException('Could not record the original user PATH state.');
        if not RegWriteStringValue(HKCU, OwnershipKey, 'PathEntry', Entry) then
          RaiseException('Could not record ownership of the Pushman PATH entry.');
        if Value = '' then WriteUserPath(Entry)
        else WriteUserPath(Entry + ';' + Value);
      end;
    end;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then RemoveOwnedPath;
end;

procedure InitializeWizard;
begin
  WizardForm.FinishedHeadingLabel.Caption := CustomMessage('FinishedHeading');
  WizardForm.FinishedLabel.Caption := CustomMessage('FinishedLabel');
end;
