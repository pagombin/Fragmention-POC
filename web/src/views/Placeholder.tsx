import { Card, CardBody } from '../components/ui/primitives';
import { Page } from './ClusterOverview';

export function Placeholder({ title, body }: { title: string; body?: string }) {
  return (
    <Page>
      <Card>
        <CardBody>
          <h2 className="text-lg font-semibold">{title}</h2>
          {body && <p className="mt-2 text-sm text-slate-500">{body}</p>}
        </CardBody>
      </Card>
    </Page>
  );
}
