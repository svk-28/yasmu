Name:           yasmu
Version:        @VERSION@
Release:        1%{?dist}
Summary:        YASMU - Yet Another System Monitoring Utility

License:        GPLv3
URL:            https://nuk-svk.ru
Source0:        %{name}-%{version}.tar.gz
Source1:        %{name}.service

BuildArch:      x86_64

%if 0%{?fedora} || 0%{?rhel} >= 8
BuildRequires: systemd-rpm-macros
%else
BuildRequires: systemd
%endif

%description
A lightweight utility for monitoring system integrity
(files, users, processes, services, ports) and website availability.
It sends alerts via email, Matrix, or webhook.

%prep
%setup -q

%build
# Бинарник уже собран, ничего не делаем.

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}%{_bindir}
mkdir -p %{buildroot}%{_unitdir}
mkdir -p %{buildroot}/etc/yasmu

install -m 0755 yasmu %{buildroot}%{_bindir}/yasmu
install -m 0644 %{SOURCE1} %{buildroot}%{_unitdir}/%{name}.service
install -m 0644 config.yaml %{buildroot}/etc/yasmu/config.yaml

%post
%systemd_post %{name}.service

%preun
%systemd_preun %{name}.service

%postun
%systemd_postun_with_restart %{name}.service

%files
%{_bindir}/yasmu
%{_unitdir}/%{name}.service
/etc/yasmu/yasmu.yaml

%changelog
* Mon Oct 05 2026 Sergey Kalinin <your@email.com> - 0.1.0-1
- Initial package
