import { useEffect, useState, useCallback } from 'react';
import {
  Button,
  EmptyState,
  EmptyStateActions,
  EmptyStateBody,
  EmptyStateFooter,
  PageSection,
  Content,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  Alert,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { useNavigate } from 'react-router-dom';
import { applications, type ApplicationRecord } from '../api/client';

export default function Applications() {
  const [apps, setApps] = useState<ApplicationRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  const load = useCallback(() => {
    setLoading(true);
    applications.list().then(setApps).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleDelete = async (name: string) => {
    if (!confirm(`Delete application "${name}"?`)) return;
    try {
      await applications.delete(name);
      load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <>
      <PageSection variant="light">
        <Content component="h1">Applications</Content>
      </PageSection>
      <PageSection>
      {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <Button onClick={() => navigate('/applications/create')}>Create application</Button>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>
      {!loading && apps.length === 0 ? (
        <EmptyState>
          <EmptyStateBody>No applications yet.</EmptyStateBody>
          <EmptyStateFooter>
            <EmptyStateActions>
              <Button onClick={() => navigate('/applications/create')}>Create application</Button>
            </EmptyStateActions>
          </EmptyStateFooter>
        </EmptyState>
      ) : (
        <Table aria-label="Applications">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Labels</Th>
              <Th>Components</Th>
              <Th>Created</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {apps.map(app => (
              <Tr
                key={app.name}
                onRowClick={() => navigate(`/applications/${app.name}`)}
                isClickable
              >
                <Td dataLabel="Name">{app.name}</Td>
                <Td dataLabel="Labels">
                  {app.labels ? Object.entries(app.labels).map(([k, v]) => `${k}=${v}`).join(', ') : '—'}
                </Td>
                <Td dataLabel="Components">{app.components.length}</Td>
                <Td dataLabel="Created">{new Date(app.createdAt).toLocaleString()}</Td>
                <Td dataLabel="Actions" isActionCell>
                  <Button variant="secondary" size="sm" onClick={e => { e.stopPropagation(); navigate(`/applications/${app.name}/edit`); }} style={{ marginRight: 8 }}>
                    Edit
                  </Button>
                  <Button variant="danger" size="sm" onClick={e => { e.stopPropagation(); handleDelete(app.name); }}>
                    Delete
                  </Button>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
    </PageSection>
    </>
  );
}
