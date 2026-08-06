Unicode true

!define REQUEST_EXECUTION_LEVEL "user"
!include "wails_tools.nsh"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "CompanyName" "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "Instalador do ${INFO_PRODUCTNAME}"
VIAddVersionKey "ProductVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion" "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright" "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName" "${INFO_PRODUCTNAME}"

ManifestDPIAware true
SetCompressor /SOLID lzma
BrandingText "Bruno Browser // Local Operations"

!include "MUI.nsh"
!include "WinVer.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_RIGHT
!define MUI_HEADERIMAGE_BITMAP "resources\headerimage.bmp"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Abrir o Bruno Browser agora"
!define MUI_WELCOMEPAGE_TITLE "Instalar o Bruno Browser"
!define MUI_WELCOMEPAGE_TEXT "Perfis persistentes, operações locais e controle de rede em uma interface segura.$\r$\n$\r$\nO assistente instalará o aplicativo somente para o seu usuário. Seus perfis e sessões permanecem no disco local."
!define MUI_DIRECTORYPAGE_TEXT_TOP "Escolha a pasta onde o Bruno Browser será instalado. Seus dados de perfis são mantidos separadamente na pasta de dados do usuário."
!define MUI_FINISHPAGE_TITLE "Bruno Browser instalado"
!define MUI_FINISHPAGE_TEXT "A instalação foi concluída. Um atalho foi criado no Menu Iniciar e na Área de Trabalho."

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "PortugueseBR"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\Bruno Browser Setup.exe"
InstallDir "$LOCALAPPDATA\Programs\Bruno Browser"
InstallDirRegKey HKCU "Software\Bruno Browser" "InstallDir"
ShowInstDetails show
ShowUninstDetails show

Function .onInit
    ${IfNot} ${AtLeastWin10}
        MessageBox MB_ICONSTOP "O Bruno Browser requer Windows 10 ou Windows 11."
        Abort
    ${EndIf}
    !insertmacro wails.checkArchitecture
FunctionEnd

Section "Bruno Browser" SEC_APP
    !insertmacro wails.setShellContext
    !insertmacro wails.webview2runtime

    SetOutPath "$INSTDIR"
    !insertmacro wails.files

    WriteRegStr HKCU "Software\Bruno Browser" "InstallDir" "$INSTDIR"
    CreateDirectory "$SMPROGRAMS\Bruno Browser"
    CreateShortcut "$SMPROGRAMS\Bruno Browser\Bruno Browser.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$DESKTOP\Bruno Browser.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    !insertmacro wails.writeUninstaller
SectionEnd

Section "Uninstall"
    !insertmacro wails.setShellContext

    Delete "$SMPROGRAMS\Bruno Browser\Bruno Browser.lnk"
    RMDir "$SMPROGRAMS\Bruno Browser"
    Delete "$DESKTOP\Bruno Browser.lnk"
    DeleteRegKey HKCU "Software\Bruno Browser"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols
    !insertmacro wails.deleteUninstaller

    RMDir /r "$INSTDIR"
SectionEnd
