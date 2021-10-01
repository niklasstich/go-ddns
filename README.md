# go-ddns - lightweight GoDaddy dyndns updater

A no nonsense DynDNS updater for your GoDaddy domains

## Configuration
Configuration is done through environment variables (e.g. docker environment variables):

| Variable      | Description                                                |
|---------------|------------------------------------------------------------|
| GD_API_KEY    | GoDaddy API Key from https://developer.godaddy.com/keys    |
| GD_API_SECRET | GoDaddy API Secret from https://developer.godaddy.com/keys |
| GD_DOMAINS    | Comma-seperated list of domains that should be updated     |
| GD_INTERVAL   | (Optional) Interval in seconds between updates        |

## Contributing
If you have any suggestions or requests, feel free to create an issue, pull request or fork!