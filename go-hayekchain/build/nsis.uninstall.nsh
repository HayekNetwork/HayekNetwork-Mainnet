Section "Uninstall"
  # uninstall for all users
  setShellVarContext all

  # Delete (optionally) installed files
  {{range $}}Delete $INSTDIR\{{.}}
  {{end}}
  Delete $INSTDIR\uninstall.exe

  # Delete install directory
  rmDir $INSTDIR

  # Delete start menu launcher
  Delete "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk"
  Delete "$SMPROGRAMS\${APPNAME}\Attach.lnk"
  Delete "$SMPROGRAMS\${APPNAME}\Uninstall.lnk"
  rmDir "$SMPROGRAMS\${APPNAME}"

  # Firewall - remove rules if exists
  SimpleFC::AdvRemoveRule "Ghyk incoming peers (TCP:30303)"
  SimpleFC::AdvRemoveRule "Ghyk outgoing peers (TCP:30303)"
  SimpleFC::AdvRemoveRule "Ghyk UDP discovery (UDP:30303)"

  # Remove IPC endpoint (https://github.com/hayekchain/EIPs/issues/147)
  ${un.EnvVarUpdate} $0 "HAYEKCHAIN_SOCKET" "R" "HKLM" "\\.\pipe\ghyk.ipc"

  # Remove install directory from PATH
  Push "$INSTDIR"
  Call un.RemoveFromPath

  # Cleanup registry (deletes all sub keys)
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${GROUPNAME} ${APPNAME}"
SectionEnd
