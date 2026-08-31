export const WORKSHOP_COLLECTION_FILE_TYPE = 2;

export const browserState = {
  page: 1,
  query: "",
  sort: "trend",
  tags: [],
  filetype: "0",
  loading: false,
  hasMore: true,
  loadFailed: false,
  // Incremented whenever the list criteria are reset. In-flight requests
  // capture this value so a late response cannot populate a newer query.
  requestVersion: 0,
  data: [],
};

export function resetWorkshopPaging() {
  browserState.requestVersion += 1;
  browserState.page = 1;
  browserState.data = [];
  browserState.hasMore = true;
  browserState.loadFailed = false;
}
