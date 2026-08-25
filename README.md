# Saihai
A self-contained line simulator and ISP for voice-capable modems.
## What this does
With the decline of landline phones and dial-up ISPs, getting hardware dependent on dial-up connectivity online has become difficult. Saihai simulates a telephone line (**separate [voltage inducer](https://youtu.be/lPvNQAi-yeM) still required**) and ISP for these devices using readily available USB modems such as the [Zoom 3095](https://www.ebay.com/sch/i.html?_nkw=zoom+3095+usb+modem) (any modem using the CX930xx chipset will work).
## Using it
Saihai will work for most users without any extra configuration, regardless of platform. Launching `saihai` will automatically detect an attached modem and accept incoming connections from it. When a call is received, Saihai initiates a connection and provides internet access through [DCNet](https://www.reddit.com/r/FlyCast/s/OA7qZ4AhcQ).
## Inspiration
Saihai was inspired by the [DreamPi](https://segaretro.org/DreamPi) project created by [Kazade](https://github.com/Kazade). No code from DreamPi is used in Saihai, but the concept of using a voice-capable modem to enable easy dial-up connectivity comes from it.
