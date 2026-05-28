import { Route as rootRoute } from './routes/__root';
import { Route as indexRoute } from './routes/index';
import { Route as contactsIndexRoute } from './routes/contacts/index';
import { Route as contactDetailRoute } from './routes/contacts/$contactId';
import { Route as companiesIndexRoute } from './routes/companies/index';
import { Route as dealsIndexRoute } from './routes/deals/index';
import { Route as dealDetailRoute } from './routes/deals/$dealId';
import { Route as settingsIndexRoute } from './routes/settings/index';
import { Route as settingsMembersRoute } from './routes/settings/members';
import { Route as reportsRoute } from './routes/reports/$workspaceId';
import { Route as pipelineRoute } from './routes/pipeline/$workspaceId';

export const routeTree = rootRoute.addChildren([
  indexRoute,
  contactsIndexRoute,
  contactDetailRoute,
  companiesIndexRoute,
  dealsIndexRoute,
  dealDetailRoute,
  settingsIndexRoute,
  settingsMembersRoute,
  reportsRoute,
  pipelineRoute,
]);
