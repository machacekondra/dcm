import { useEffect, useState, useCallback } from 'react';
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
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { applications, type ApplicationRecord } from '../api/client';

export default function ApplicationDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const [app, setApp] = useState<ApplicationRecord | null>(null);
  const [error, setError] = useState('');

  const load = useCallback(() => {
    if (!name) return;
    applications.get(name).then(setApp).catch(e => setError(e.message));
  }, [name]);

  useEffect(() => { load(); }, [load]);

  const handleDelete = async () => {
    if (!name || !confirm(`Delete application "${name}"?`)) return;
    try {
      await applications.delete(name);
      navigate('/applications');
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  if (error) {
    return (
      <PageSection>
        <Alert variant="danger" title={error} />
      </PageSection>
    );
  }
  if (!app) {
    return <PageSection><Spinner /></PageSection>;
  }

  return (
    <>
      <PageSection variant="light">
        <Breadcrumb style={{ marginBottom: 16 }}>
          <BreadcrumbItem to="/applications" onClick={e => { e.preventDefault(); navigate('/applications'); }}>
            Applications
          </BreadcrumbItem>
          <BreadcrumbItem isActive>{name}</BreadcrumbItem>
        </Breadcrumb>

        <Flex justifyContent={{ default: 'justifyContentSpaceBetween' }} alignItems={{ default: 'alignItemsCenter' }}>
          <FlexItem>
            <Content component="h1">{app.name}</Content>
          </FlexItem>
          <FlexItem>
            <Button variant="secondary" onClick={() => navigate(`/applications/${name}/edit`)} style={{ marginRight: 8 }}>Edit</Button>
            <Button variant="danger" onClick={handleDelete}>Delete</Button>
          </FlexItem>
        </Flex>
      </PageSection>
      <PageSection>
      <Card style={{ marginBottom: 24 }}>
        <CardTitle>Details</CardTitle>
        <CardBody>
          <DescriptionList isHorizontal>
            <DescriptionListGroup>
              <DescriptionListTerm>Name</DescriptionListTerm>
              <DescriptionListDescription>{app.name}</DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Labels</DescriptionListTerm>
              <DescriptionListDescription>
                {app.labels && Object.keys(app.labels).length > 0 ? (
                  <LabelGroup>
                    {Object.entries(app.labels).map(([k, v]) => (
                      <Label key={k}>{k}={v}</Label>
                    ))}
                  </LabelGroup>
                ) : '—'}
              </DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Created</DescriptionListTerm>
              <DescriptionListDescription>{new Date(app.createdAt).toLocaleString()}</DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Updated</DescriptionListTerm>
              <DescriptionListDescription>{new Date(app.updatedAt).toLocaleString()}</DescriptionListDescription>
            </DescriptionListGroup>
          </DescriptionList>
        </CardBody>
      </Card>

      <Card>
        <CardTitle>Components ({app.components.length})</CardTitle>
        <CardBody>
          <Table aria-label="Components" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Type</Th>
                <Th>Dependencies</Th>
                <Th>Labels</Th>
                <Th>Properties</Th>
              </Tr>
            </Thead>
            <Tbody>
              {app.components.map(c => (
                <Tr key={c.name}>
                  <Td dataLabel="Name"><strong>{c.name}</strong></Td>
                  <Td dataLabel="Type"><Label isCompact>{c.type}</Label></Td>
                  <Td dataLabel="Dependencies">{c.dependsOn?.join(', ') || '—'}</Td>
                  <Td dataLabel="Labels">
                    {c.labels ? Object.entries(c.labels).map(([k, v]) => `${k}=${v}`).join(', ') : '—'}
                  </Td>
                  <Td dataLabel="Properties">
                    {c.properties ? (
                      <code style={{ fontSize: 12 }}>{JSON.stringify(c.properties)}</code>
                    ) : '—'}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </CardBody>
      </Card>
    </PageSection>
    </>
  );
}
