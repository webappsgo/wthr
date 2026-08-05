(function () {
  "use strict";

  var container = document.getElementById("graphiql");
  var endpoint = container.dataset.endpoint || "/graphql";

  var url = location.protocol + "//" + location.host + endpoint;
  var wsProto = location.protocol === "https:" ? "wss:" : "ws:";
  var subscriptionUrl = wsProto + "//" + location.host + endpoint;

  var fetcher = GraphiQL.createFetcher({ url: url, subscriptionUrl: subscriptionUrl });

  ReactDOM.render(
    React.createElement(GraphiQL, {
      fetcher: fetcher,
      isHeadersEditorEnabled: true,
      shouldPersistHeaders: true
    }),
    container
  );
})();
