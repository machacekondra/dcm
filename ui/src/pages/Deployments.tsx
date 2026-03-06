import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  EmptyState,
  Flex,
  FlexItem,
  EmptyStateActions,
  EmptyStateBody,
  EmptyStateFooter,
  FormGroup,
  MenuToggle,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  PageSection,
  Content,
  Select,
  SelectList,
  SelectOption,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { useNavigate } from 'react-router-dom';
import { deployments, applications, type DeploymentRecord, type ApplicationRecord } from '../api/client';
import StatusLabel from '../components/StatusLabel';

const TERMINAL_STATUSES = ['destroyed', 'failed'];

export default function Deployments() {
  const [list, setList] = useState<DeploymentRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [isDeployOpen, setDeployOpen] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  const load = useCallback(() => {
    setLoading(true);
    deployments.list().then(setList).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleDelete = async (e: React.MouseEvent, d: DeploymentRecord) => {
    e.stopPropagation();
    const action = d.status === 'ready' ? 'Destroy' : 'Delete';
    if (!confirm(`${action} deployment "${d.id.slice(0, 16)}" (${d.application})?`)) return;
    try {
      await deployments.destroy(d.id);
      load();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  // Auto-refresh if any deployment is in progress.
  useEffect(() => {
    const hasActive = list.some(d => ['pending', 'planning', 'deploying', 'destroying'].includes(d.status));
    if (!hasActive) return;
    const id = setInterval(load, 3000);
    return () => clearInterval(id);
  }, [list, load]);

  return (
    <>
      <PageSection variant="light">
        <Content component="h1">Deployments</Content>
      </PageSection>
      <PageSection>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Button onClick={() => setDeployOpen(true)}>Deploy application</Button>
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="secondary" onClick={load}>Refresh</Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
        {!loading && list.length === 0 ? (
          <EmptyState>
            <EmptyStateBody>No deployments yet.</EmptyStateBody>
            <EmptyStateFooter>
              <EmptyStateActions>
                <Button onClick={() => setDeployOpen(true)}>Deploy application</Button>
              </EmptyStateActions>
            </EmptyStateFooter>
          </EmptyState>
        ) : (
          <Table aria-label="Deployments">
            <Thead>
              <Tr>
                <Th>ID</Th>
                <Th>Application</Th>
                <Th>Status</Th>
                <Th>Created</Th>
                <Th>Actions</Th>
              </Tr>
            </Thead>
            <Tbody>
              {list.map(d => (
                <Tr key={d.id} onRowClick={() => navigate(`/deployments/${d.id}`)} isClickable>
                  <Td dataLabel="ID"><code>{d.id.slice(0, 16)}</code></Td>
                  <Td dataLabel="Application">{d.application}</Td>
                  <Td dataLabel="Status"><StatusLabel status={d.status} /></Td>
                  <Td dataLabel="Created">{new Date(d.createdAt).toLocaleString()}</Td>
                  <Td dataLabel="Actions" isActionCell>
                    <Flex gap={{ default: 'gapSm' }}>
                      {d.status === 'planned' && (
                        <FlexItem>
                          <Button variant="primary" size="sm" onClick={async e => {
                            e.stopPropagation();
                            try { await deployments.apply(d.id); load(); } catch (err: unknown) { setError(err instanceof Error ? err.message : String(err)); }
                          }}>Apply</Button>
                        </FlexItem>
                      )}
                      {(d.status === 'ready' || d.status === 'planned' || TERMINAL_STATUSES.includes(d.status)) && (
                        <FlexItem>
                          <Button variant="danger" size="sm" onClick={e => handleDelete(e, d)}>
                            {d.status === 'ready' ? 'Destroy' : 'Delete'}
                          </Button>
                        </FlexItem>
                      )}
                    </Flex>
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
        <DeployModal isOpen={isDeployOpen} onClose={() => setDeployOpen(false)} onDeployed={load} />
      </PageSection>
    </>
  );
}

function DeployModal({ isOpen, onClose, onDeployed }: { isOpen: boolean; onClose: () => void; onDeployed: () => void }) {
  const [apps, setApps] = useState<ApplicationRecord[]>([]);
  const [selectedApp, setSelectedApp] = useState('');
  const [dryRun, setDryRun] = useState(false);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [appSelectOpen, setAppSelectOpen] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    if (!isOpen) return;
    applications.list().then(setApps).catch(() => {});
    setSelectedApp('');
    setDryRun(false);
    setError('');
  }, [isOpen]);

  const handleSubmit = async () => {
    setError('');
    setSubmitting(true);
    try {
      const dep = await deployments.create({
        application: selectedApp,
        dryRun,
      });
      onClose();
      onDeployed();
      navigate(`/deployments/${dep.id}`);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} variant="medium">
      <ModalHeader title="Deploy Application" />
      <ModalBody>
        {error && <Alert variant="danger" title={error} isInline style={{ marginBottom: 16 }} />}
        <FormGroup label="Application" isRequired fieldId="deploy-app" style={{ marginBottom: 16 }}>
          <Select
            id="deploy-app"
            isOpen={appSelectOpen}
            selected={selectedApp}
            onSelect={(_e, val) => { setSelectedApp(val as string); setAppSelectOpen(false); }}
            onOpenChange={setAppSelectOpen}
            toggle={(toggleRef) => (
              <MenuToggle ref={toggleRef} onClick={() => setAppSelectOpen(!appSelectOpen)} isExpanded={appSelectOpen} style={{ width: '100%' }}>
                {selectedApp || 'Select application'}
              </MenuToggle>
            )}
          >
            <SelectList>
              {apps.map(a => (
                <SelectOption key={a.name} value={a.name}>{a.name}</SelectOption>
              ))}
            </SelectList>
          </Select>
        </FormGroup>

        <FormGroup fieldId="deploy-dryrun">
          <Checkbox
            id="deploy-dryrun"
            label="Dry run (plan only, don't apply)"
            isChecked={dryRun}
            onChange={(_e, checked) => setDryRun(checked)}
          />
        </FormGroup>
      </ModalBody>
      <ModalFooter>
        <Button onClick={handleSubmit} isLoading={submitting} isDisabled={!selectedApp || submitting}>
          {dryRun ? 'Plan' : 'Deploy'}
        </Button>
        <Button variant="link" onClick={onClose}>Cancel</Button>
      </ModalFooter>
    </Modal>
  );
}
