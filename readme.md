## Requirements
The bot requires the patch in order to make parsing of the logs easier: 
[Issue @Teeworlds](https://github.com/teeworlds/teeworlds/issues/2475#issuecomment-590102555)

## Build
```
go get -d
go build
```

## Example configuration
The admins ID needs to be explicitly in quotation marks, as the parsing of the config file will fail otherwise.
It is possible to use one password with many econ connections or to specify one password, delimited by one whitespace ` ` for each econ address.
This example configuration must live in the same folder as the executable and needs to be called `.env`

Log level:
 -  0 : chat & votes & rcon,  
 -  1: 0 & join & leave
 -  2: 1 & whisper // TODO
```
DISCORD_TOKEN=1234567890
DISCORD_ADMIN="nickname#1234"

DISCORD_MODERATORS="nickname#12345 nickname2#3456"
DISCORD_MODERATOR_ROLE="Server Moderator"
DISCORD_MODERATOR_COMMANDS="status vote say mute unmute mutes voteban unvoteban unvoteban_client votebans ban unban bans set_team"

ECON_SERVERS=127.0.0.1:9303 127.0.0.1:9304
ECON_PASSWORDS=abcdefghijkl

LOG_LEVEL=0
```


## Usage
 - Start the bot
 - Add your bot to a server/channel
 - execute `#moderate 127.0.0.1:9303` in any channel that the bot has access to. That channel becommes the bot's log channel. Repeated executions of that command will not work, thus forcing you to restart the bot if a new channel should be used as the log channel.
 - the bot tries to hide as much personal info, especially ips.
 - after the bot started, you can use commands like `?help`, `?status`, `?bans`, `?mutes`, `?votebans` and many more.
 - the output of the explicitly written down commands is visible in discord.
 - if you want to add moderators, use the commands:
   - `#add nickname#1234` to give a player access to the commands
   - `#remove nickname#1234`to remove a specific player
   - `#purge`to remove all moderators and admins except for the one specified in the `.env` file.