import {
  Brand,
  Masthead,
  MastheadBrand,
  MastheadLogo,
  MastheadMain,
  Nav,
  NavItem,
  NavList,
  Page,
  PageSidebar,
  PageSidebarBody,
} from '@patternfly/react-core';
import { NavLink, Outlet, useLocation } from 'react-router-dom';

const navItems = [
  { to: '/deployments', label: 'Deployments' },
  { to: '/applications', label: 'Applications' },
  { to: '/policies', label: 'Policies' },
  { to: '/environments', label: 'Environments' },
  { to: '/providers', label: 'Providers' },
];

export default function Layout() {
  const location = useLocation();

  const header = (
    <Masthead>
      <MastheadMain>
        <MastheadBrand>
          <MastheadLogo component="span">
            <span style={{ fontWeight: 700, fontSize: 18, color: 'var(--pf-t--global--color--brand--default)' }}>DCM</span>
            <span style={{ marginLeft: 8, fontSize: 14, color: 'var(--pf-t--global--text--color--subtle)' }}>Declarative Cloud Manager</span>
          </MastheadLogo>
        </MastheadBrand>
      </MastheadMain>
    </Masthead>
  );

  const sidebar = (
    <PageSidebar>
      <PageSidebarBody>
        <Nav>
          <NavList>
            {navItems.map(({ to, label }) => (
              <NavItem key={to} isActive={location.pathname.startsWith(to)}>
                <NavLink to={to}>{label}</NavLink>
              </NavItem>
            ))}
          </NavList>
        </Nav>
      </PageSidebarBody>
    </PageSidebar>
  );

  return (
    <Page masthead={header} sidebar={sidebar} isManagedSidebar>
      <Outlet />
    </Page>
  );
}
