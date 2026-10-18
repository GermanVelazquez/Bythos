# Spec: installer-firewall (bythos-movil-qr)

## Purpose
Installer requests admin privileges, manages a scoped firewall rule for the LAN listener, and migrates prior per-user installs.

## ADDED Requirements

### Requirement: Admin Install and Firewall Rule
The installer MUST run elevated and add an inbound allow rule for `bythos.exe` scoped exclusively to the Private network profile and the local subnet, removed on uninstall. No rule permitting the Public profile MUST ever be created.

#### Scenario: Fresh install
- GIVEN a machine with no prior install
- WHEN setup completes
- THEN the firewall rule exists scoped exclusively to the Private profile and local subnet, with no Public-profile rule created

#### Scenario: Uninstall
- GIVEN an installed rule
- WHEN the app is uninstalled
- THEN the firewall rule is removed

### Requirement: Install-Scope Migration
The installer MUST detect an existing per-user (HKCU) install and migrate to per-machine, preserving `%APPDATA%` data.

#### Scenario: Migration
- GIVEN a prior per-user install
- WHEN the per-machine installer runs
- THEN the old registration is silently uninstalled, the new per-machine install proceeds, and user data is preserved
