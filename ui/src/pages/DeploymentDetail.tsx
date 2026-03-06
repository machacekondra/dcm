import { useEffect, useState, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Alert,
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Card,
  CardBody,
  CardTitle,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Flex,
  FlexItem,
  Label,
  LabelGroup,
  PageSection,
  Spinner,
  Tab,
  Tabs,
  TabTitleText,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { deployments, type DeploymentRecord, type HistoryRecord } from '../api/client';
import StatusLabel from '../components/StatusLabel';

const ACTIVE_STATUSES = ['pending', 'planning', 'deploying', 'destroying'];

export default function DeploymentDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [dep, setDep] = useState<DeploymentRecord | null>(null);
  const [history, setHistory] = useState<HistoryRecord[]>([]);
  const [activeTab, setActiveTab] = useState<string | number>('overview');
  const [error, setError] = useState('');
  const intervalRef = useRef<number>();

  const load = useCallback(() => {
    if (!id) return;
    deployments.get(id).then(setDep).catch(e => setError(e.message));
    deployments.history(id).then(setHistory).catch(() => {});
  }, [id]);

  useEffect(() => { load(); }, [load]);

  // Auto-poll while deployment is in progress.
  useEffect(() => {
    if (dep && ACTIVE_STATUSES.includes(dep.status)) {
      intervalRef.current = window.setInterval(load, 2000);
    } else if (intervalRef.current) {
      clearInterval(intervalRef.current);
    }
    return () => { if (intervalRef.current) clearInterval(intervalRef.current); };
  }, [dep?.status, load]);

  const handleDestroy = async () => {
    if (!id || !confirm('Destroy this deployment? All resources will be removed.')) return;
    try {
      await deployments.destroy(id);
      load();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  if (error && !dep) return <PageSection><Alert variant="danger" title={error} /></PageSection>;
  if (!dep) return <PageSection><Spinner /></PageSection>;

  const isActive = ACTIVE_STATUSES.includes(dep.status);

  return (
    <>
      <PageSection variant="light">
        <Breadcrumb style={{ marginBottom: 16 }}>
          <BreadcrumbItem to="/deployments" onClick={e => { e.preventDefault(); navigate('/deployments'); }}>
            Deployments
          </BreadcrumbItem>
          <BreadcrumbItem isActive>{id?.slice(0, 16)}</BreadcrumbItem>
        </Breadcrumb>

        <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
          <FlexItem>
            <Flex gap={{ default: 'gapMd' }} alignItems={{ default: 'alignItemsCenter' }}>
              <FlexItem><Content component="h1" style={{ margin: 0 }}>{dep.application}</Content></FlexItem>
              <FlexItem><StatusLabel status={dep.status} /></FlexItem>
              {isActive && <FlexItem><Spinner size="md" /></FlexItem>}
            </Flex>
          </FlexItem>
          <FlexItem>
            {(dep.status === 'ready' || dep.status === 'failed') && (
              <Button variant="danger" onClick={handleDestroy}>Destroy</Button>
            )}
          </FlexItem>
        </Flex>
      </PageSection>
      <PageSection>
      {dep.error && <Alert variant="danger" title="Deployment failed" isInline style={{ marginBottom: 16 }}>{dep.error}</Alert>}
      {error && <Alert variant="warning" title={error} isInline style={{ marginBottom: 16 }} />}

      <Tabs activeKey={activeTab} onSelect={(_e, key) => setActiveTab(key)}>
        <Tab eventKey="overview" title={<TabTitleText>Overview</TabTitleText>}>
          <Card style={{ marginTop: 16 }}>
            <CardBody>
              <DescriptionList isHorizontal>
                <DescriptionListGroup>
                  <DescriptionListTerm>ID</DescriptionListTerm>
                  <DescriptionListDescription><code>{dep.id}</code></DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Application</DescriptionListTerm>
                  <DescriptionListDescription>{dep.application}</DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Status</DescriptionListTerm>
                  <DescriptionListDescription><StatusLabel status={dep.status} /></DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Policies</DescriptionListTerm>
                  <DescriptionListDescription>
                    {dep.policies?.length ? (
                      <LabelGroup>{dep.policies.map(p => <Label key={p}>{p}</Label>)}</LabelGroup>
                    ) : '—'}
                  </DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Created</DescriptionListTerm>
                  <DescriptionListDescription>{new Date(dep.createdAt).toLocaleString()}</DescriptionListDescription>
                </DescriptionListGroup>
                <DescriptionListGroup>
                  <DescriptionListTerm>Updated</DescriptionListTerm>
                  <DescriptionListDescription>{new Date(dep.updatedAt).toLocaleString()}</DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>
        </Tab>

        <Tab eventKey="plan" title={<TabTitleText>Plan {dep.plan ? `(${dep.plan.steps.length})` : ''}</TabTitleText>}>
          <Card style={{ marginTop: 16 }}>
            <CardBody>
              {dep.plan ? (
                <Table aria-label="Plan steps" variant="compact">
                  <Thead>
                    <Tr>
                      <Th>Component</Th>
                      <Th>Action</Th>
                      <Th>Type</Th>
                      <Th>Provider</Th>
                      <Th>Environment</Th>
                      <Th>Matched Rules</Th>
                    </Tr>
                  </Thead>
                  <Tbody>
                    {dep.plan.steps.map(step => (
                      <Tr key={step.component}>
                        <Td dataLabel="Component"><strong>{step.component}</strong></Td>
                        <Td dataLabel="Action">
                          <Label
                            isCompact
                            color={step.diff.action === 'create' ? 'green' : step.diff.action === 'delete' ? 'red' : step.diff.action === 'update' ? 'blue' : 'grey'}
                          >
                            {step.diff.action}
                          </Label>
                        </Td>
                        <Td dataLabel="Type">{step.diff.type}</Td>
                        <Td dataLabel="Provider">{step.diff.provider}</Td>
                        <Td dataLabel="Environment">{step.diff.environment || '—'}</Td>
                        <Td dataLabel="Matched Rules">
                          {step.matchedRules?.length ? (
                            <LabelGroup>{step.matchedRules.map(r => <Label key={r} isCompact color="cyan">{r}</Label>)}</LabelGroup>
                          ) : '—'}
                        </Td>
                      </Tr>
                    ))}
                  </Tbody>
                </Table>
              ) : (
                <Content component="p">No plan available yet.</Content>
              )}
            </CardBody>
          </Card>
        </Tab>

        <Tab eventKey="resources" title={<TabTitleText>Resources {dep.state ? `(${Object.keys(dep.state.resources).length})` : ''}</TabTitleText>}>
          <Card style={{ marginTop: 16 }}>
            <CardBody>
              {dep.state && Object.keys(dep.state.resources).length > 0 ? (
                Object.entries(dep.state.resources).map(([name, resource]) => (
                  <Card key={name} isCompact style={{ marginBottom: 12 }}>
                    <CardTitle>
                      <Flex gap={{ default: 'gapSm' }} alignItems={{ default: 'alignItemsCenter' }}>
                        <FlexItem>{name}</FlexItem>
                        <FlexItem><Label isCompact>{resource.type}</Label></FlexItem>
                        <FlexItem><Label isCompact color="blue">{resource.provider}</Label></FlexItem>
                        <FlexItem><StatusLabel status={resource.status} /></FlexItem>
                      </Flex>
                    </CardTitle>
                    <CardBody>
                      {resource.outputs && Object.keys(resource.outputs).length > 0 && (
                        <DescriptionList isHorizontal isCompact>
                          {Object.entries(resource.outputs).map(([k, v]) => (
                            <DescriptionListGroup key={k}>
                              <DescriptionListTerm>{k}</DescriptionListTerm>
                              <DescriptionListDescription>
                                <code style={{ fontSize: 12 }}>{String(v)}</code>
                              </DescriptionListDescription>
                            </DescriptionListGroup>
                          ))}
                        </DescriptionList>
                      )}
                    </CardBody>
                  </Card>
                ))
              ) : (
                <Content component="p">No resources deployed yet.</Content>
              )}
            </CardBody>
          </Card>
        </Tab>

        <Tab eventKey="history" title={<TabTitleText>History ({history.length})</TabTitleText>}>
          <Card style={{ marginTop: 16 }}>
            <CardBody>
              {history.length > 0 ? (
                <Table aria-label="History" variant="compact">
                  <Thead>
                    <Tr>
                      <Th>Time</Th>
                      <Th>Action</Th>
                      <Th>Details</Th>
                    </Tr>
                  </Thead>
                  <Tbody>
                    {history.map(h => (
                      <Tr key={h.id}>
                        <Td dataLabel="Time">{new Date(h.createdAt).toLocaleString()}</Td>
                        <Td dataLabel="Action">
                          <Label
                            isCompact
                            color={
                              h.action === 'applied' || h.action === 'destroyed' ? 'green'
                                : h.action === 'failed' ? 'red'
                                : h.action === 'planning' || h.action === 'destroying' ? 'orange'
                                : 'blue'
                            }
                          >
                            {h.action}
                          </Label>
                        </Td>
                        <Td dataLabel="Details">
                          {h.details ? (
                            <details>
                              <summary style={{ cursor: 'pointer' }}>Show details</summary>
                              <pre style={{ fontSize: 11, maxHeight: 200, overflow: 'auto', marginTop: 8 }}>
                                {JSON.stringify(h.details, null, 2)}
                              </pre>
                            </details>
                          ) : '—'}
                        </Td>
                      </Tr>
                    ))}
                  </Tbody>
                </Table>
              ) : (
                <Content component="p">No history yet.</Content>
              )}
            </CardBody>
          </Card>
        </Tab>
      </Tabs>
    </PageSection>
    </>
  );
}
